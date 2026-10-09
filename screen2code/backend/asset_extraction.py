import asyncio
import base64
import io
import json
import math
import time
from collections.abc import Callable
from dataclasses import dataclass, field
from typing import Any, Dict, List, Sequence, cast

import pillow_heif
from google import genai
from google.genai import types
from PIL import Image, ImageOps
from pydantic import BaseModel, Field, ValidationError


ASSET_EXTRACTION_GEMINI_MODEL = "gemini-3.6-flash"
MAX_ASSETS_PER_GEMINI_REQUEST = 25
DEFAULT_ASSET_MEDIA_RESOLUTION = (
    types.PartMediaResolutionLevel.MEDIA_RESOLUTION_ULTRA_HIGH
)
SUPPORTED_GEMINI_IMAGE_MIME_TYPES = frozenset(
    {
        "image/png",
        "image/jpeg",
        "image/webp",
        "image/heic",
        "image/heif",
    }
)

# Gemini accepts HEIC/HEIF, but Pillow needs its libheif-backed plugin to decode
# those pixels before EXIF normalization and cropping. pillow-heif's public
# function currently lacks a complete type annotation, so narrow it here.
_register_heif_opener = cast(Callable[[], None], pillow_heif.register_heif_opener)
_register_heif_opener()


class AssetDetection(BaseModel):
    """One schema-constrained answer, correlated by our stable request ID."""

    request_id: str = Field(
        description="对应输入请求中保持不变的 request_id。"
    )
    image_index: int | None = Field(
        description=(
            "包含目标素材的源图编号，从 1 开始；目标不存在、无法明确区分或无法单独提取时返回 null。"
        )
    )
    box_2d: list[float] | None = Field(
        description=(
            "紧密包围素材的 [ymin, xmin, ymax, xmax] 边界，归一化至 0–1000；image_index 为 null 时此项也为 null。"
        ),
        min_length=4,
        max_length=4,
    )
    label: str | None = Field(
        description=(
            "可区分此素材与相似副本的简短中文标签；未找到素材时返回 null。"
        )
    )


class AssetDetectionBatch(BaseModel):
    detections: list[AssetDetection] = Field(
        description="每个请求的 request_id 必须恰好对应一条检测结果。",
        max_length=MAX_ASSETS_PER_GEMINI_REQUEST,
    )


@dataclass(frozen=True)
class SourceImage:
    part: types.Part
    image: Image.Image
    mime_type: str
    image_index: int


@dataclass(frozen=True)
class AssetRequest:
    request_id: str
    description: str


def _empty_float_list() -> list[float]:
    return []


def _empty_string_list() -> list[str]:
    return []


@dataclass
class AssetExtractionMetrics:
    """Optional request telemetry used by the opt-in live benchmark."""

    request_count: int = 0
    prompt_token_count: int = 0
    candidate_token_count: int = 0
    thoughts_token_count: int = 0
    total_token_count: int = 0
    request_latencies_seconds: list[float] = field(
        default_factory=_empty_float_list
    )
    response_ids: list[str] = field(default_factory=_empty_string_list)

    def record_response(
        self,
        response: types.GenerateContentResponse,
        latency_seconds: float,
    ) -> None:
        self.request_latencies_seconds.append(latency_seconds)
        if response.response_id:
            self.response_ids.append(response.response_id)

        usage = response.usage_metadata
        if usage is None:
            return
        self.prompt_token_count += usage.prompt_token_count or 0
        self.candidate_token_count += usage.candidates_token_count or 0
        self.thoughts_token_count += usage.thoughts_token_count or 0
        self.total_token_count += usage.total_token_count or 0


ASSET_DETECTION_SYSTEM_INSTRUCTION = """你是用于截图素材提取的精确二维目标检测器。
只返回一个符合指定结构的 JSON 对象，不得返回蒙版、分割数据、Markdown 代码围栏、说明文字或虚构检测结果。每次响应最多处理 25 个请求对象。
"""


def _normalize_image_for_detection(image: Image.Image) -> Image.Image:
    """Apply EXIF orientation once and produce stable PNG-compatible pixels."""

    oriented = ImageOps.exif_transpose(image)
    has_alpha = "A" in oriented.getbands() or "transparency" in oriented.info
    normalized = oriented.convert("RGBA" if has_alpha else "RGB")
    normalized.load()
    return normalized


def _data_url_to_source_image(
    data_url: str,
    *,
    image_index: int = 1,
    media_resolution: types.PartMediaResolutionLevel = DEFAULT_ASSET_MEDIA_RESOLUTION,
) -> SourceImage | None:
    if not data_url.startswith("data:image/") or "," not in data_url:
        return None

    header, encoded = data_url.split(",", 1)
    mime_type = header.removeprefix("data:").split(";", 1)[0].lower()
    if mime_type not in SUPPORTED_GEMINI_IMAGE_MIME_TYPES:
        return None

    try:
        image_bytes = base64.b64decode(encoded, validate=True)
        with Image.open(io.BytesIO(image_bytes)) as opened_image:
            opened_image.seek(0)
            normalized_image = _normalize_image_for_detection(opened_image)
    except Exception:
        return None

    # Gemini must see the exact same oriented pixel matrix that we later crop.
    # Canonical PNG also prevents a misleading/unsupported data-URL MIME header
    # from putting model coordinates in a different orientation than Pillow.
    normalized_output = io.BytesIO()
    normalized_image.save(normalized_output, format="PNG")
    normalized_bytes = normalized_output.getvalue()
    normalized_mime_type = "image/png"

    return SourceImage(
        part=types.Part.from_bytes(
            data=normalized_bytes,
            mime_type=normalized_mime_type,
            media_resolution=media_resolution,
        ),
        image=normalized_image,
        mime_type=normalized_mime_type,
        image_index=image_index,
    )


def _stable_request_id(original_index: int) -> str:
    return f"asset-{original_index + 1:04d}"


def _chunk_requests(
    requests: Sequence[AssetRequest],
) -> list[list[AssetRequest]]:
    return [
        list(requests[start : start + MAX_ASSETS_PER_GEMINI_REQUEST])
        for start in range(0, len(requests), MAX_ASSETS_PER_GEMINI_REQUEST)
    ]


def _build_detection_prompt(
    source_images: Sequence[SourceImage],
    requests: Sequence[AssetRequest],
) -> str:
    source_mapping = "\n".join(
        f"- 附件图片 {attachment_index} = 源图 {source.image_index} "
        f"（EXIF 方向归一化后为 {source.image.width}x{source.image.height} 像素）"
        for attachment_index, source in enumerate(source_images, start=1)
    )
    request_json = json.dumps(
        [
            {
                "request_id": request.request_id,
                "description": request.description,
            }
            for request in requests
        ],
        ensure_ascii=False,
        indent=2,
    )

    return f"""请在附件源图中定位每个请求的视觉素材。

源图映射（image_index 必须使用从 1 开始的源图编号）：
{source_mapping}

请求：
{request_json}

边界框规则：
- 每个请求恰好返回一条 detections 记录，保持 request_id 不变。各请求独立，不依靠输出顺序匹配。
- box_2d 为 [ymin, xmin, ymax, xmax]，相对于所选源图，归一化至 0–1000。
- 返回包含整个可见目标素材的最小轴对齐边界框，覆盖全部可见边缘与像素，不得裁掉目标。
- 排除周围的界面：卡片、容器、页面背景、留白、边框、附近文字、说明、控件及无关阴影。仅当描述明确要求时才包含这些内容。
- 照片或插画只框住可见媒体本身，不含卡片或说明。徽标/文字标识只包含完整标识。小图标只包含图形，除非明确要求，否则不包含按钮、容器或徽章。
- 重复的相似素材是不同实例。依据描述的外观、位置、附近上下文和源图编号选择精确实例，不要复用或合并边界框；标签应能区分重复实例。
- 目标不存在、无法唯一识别、大面积遮挡或无法与无关界面分离时，返回该 request_id，并令 image_index=null、box_2d=null、label=null。不得猜测边界框、用相似素材代替或返回整张截图。
- 只按所提供结构返回 JSON，不得返回蒙版、多边形、代码围栏或解释文字。
"""


def _parse_detection_response(
    response: types.GenerateContentResponse,
) -> AssetDetectionBatch:
    parsed = response.parsed
    try:
        if isinstance(parsed, AssetDetectionBatch):
            return parsed
        if parsed is not None:
            return AssetDetectionBatch.model_validate(parsed)
        if response.text:
            return AssetDetectionBatch.model_validate_json(response.text)
    except (ValidationError, TypeError, ValueError):
        pass
    return AssetDetectionBatch(detections=[])


async def _locate_asset_batch(
    client: genai.Client,
    source_images: Sequence[SourceImage],
    requests: Sequence[AssetRequest],
    metrics: AssetExtractionMetrics | None = None,
) -> AssetDetectionBatch:
    prompt = _build_detection_prompt(source_images, requests)
    if metrics is not None:
        metrics.request_count += 1
    started_at = time.perf_counter()
    response = await client.aio.models.generate_content(
        model=ASSET_EXTRACTION_GEMINI_MODEL,
        contents=[
            types.Content(
                role="user",
                # Keep the task prompt after all image parts, as recommended for
                # image understanding. The prompt maps attachment order back to
                # the caller's original 1-based source image numbers.
                parts=[source.part for source in source_images]
                + [types.Part(text=prompt)],
            )
        ],
        config=types.GenerateContentConfig(
            system_instruction=ASSET_DETECTION_SYSTEM_INSTRUCTION,
            temperature=0.5,
            response_mime_type="application/json",
            response_schema=AssetDetectionBatch,
            thinking_config=types.ThinkingConfig(
                thinking_level=types.ThinkingLevel.MINIMAL
            ),
        ),
    )
    if metrics is not None:
        metrics.record_response(response, time.perf_counter() - started_at)
    return _parse_detection_response(response)


def _normalize_box(value: object) -> list[float] | None:
    if not isinstance(value, list) or len(value) != 4:
        return None

    coordinates: list[float] = []
    for coordinate in cast(list[object], value):
        if isinstance(coordinate, bool) or not isinstance(coordinate, (int, float)):
            return None
        number = float(coordinate)
        if not math.isfinite(number):
            return None
        coordinates.append(number)

    raw_ymin, raw_xmin, raw_ymax, raw_xmax = coordinates
    ymin, ymax = sorted((raw_ymin, raw_ymax))
    xmin, xmax = sorted((raw_xmin, raw_xmax))
    ymin = max(0.0, min(1000.0, ymin))
    xmin = max(0.0, min(1000.0, xmin))
    ymax = max(0.0, min(1000.0, ymax))
    xmax = max(0.0, min(1000.0, xmax))

    if ymax <= ymin or xmax <= xmin:
        return None
    return [ymin, xmin, ymax, xmax]


def _crop_box_to_data_url(image: Image.Image, box_2d: list[float]) -> str | None:
    normalized_box = _normalize_box(box_2d)
    if normalized_box is None:
        return None

    width, height = image.size
    ymin, xmin, ymax, xmax = normalized_box
    # Outward rounding preserves every edge pixel instead of truncating the
    # model's bottom/right bounds and accidentally shaving a one-pixel border.
    left = max(0, min(width, math.floor(xmin / 1000 * width)))
    top = max(0, min(height, math.floor(ymin / 1000 * height)))
    right = max(0, min(width, math.ceil(xmax / 1000 * width)))
    bottom = max(0, min(height, math.ceil(ymax / 1000 * height)))

    if right <= left or bottom <= top:
        return None

    cropped = image.crop((left, top, right, bottom))
    if cropped.width <= 0 or cropped.height <= 0:
        return None

    output = io.BytesIO()
    cropped.save(output, format="PNG")
    encoded = base64.b64encode(output.getvalue()).decode("ascii")
    return f"data:image/png;base64,{encoded}"


async def extract_assets_from_images(
    image_data_urls: List[str],
    asset_descriptions: List[str],
    gemini_api_key: str,
    *,
    media_resolution: types.PartMediaResolutionLevel = DEFAULT_ASSET_MEDIA_RESOLUTION,
    metrics: AssetExtractionMetrics | None = None,
) -> Dict[str, Any]:
    source_images: list[SourceImage] = []
    for image_index, data_url in enumerate(image_data_urls, start=1):
        source = _data_url_to_source_image(
            data_url,
            image_index=image_index,
            media_resolution=media_resolution,
        )
        if source is not None:
            source_images.append(source)

    if not source_images:
        return {
            "assets": [],
            "error": "没有可用于提取素材的有效输入图片。",
        }

    requests = [
        AssetRequest(
            request_id=_stable_request_id(original_index),
            description=description,
        )
        for original_index, description in enumerate(asset_descriptions)
    ]
    if not requests:
        return {"assets": []}

    # Per-part media resolution is a Gemini 3 v1alpha feature. Batches are
    # independent, so requests above the Cookbook's 25-object cap can run in
    # parallel while still being mapped deterministically by stable IDs.
    client = genai.Client(
        api_key=gemini_api_key,
        http_options=types.HttpOptions(api_version="v1alpha"),
    )
    request_chunks = _chunk_requests(requests)
    batch_results = await asyncio.gather(
        *(
            _locate_asset_batch(client, source_images, chunk, metrics)
            for chunk in request_chunks
        )
    )

    detections_by_request_id: dict[str, AssetDetection] = {}
    for request_chunk, batch in zip(request_chunks, batch_results):
        expected_ids = {request.request_id for request in request_chunk}
        for detection in batch.detections:
            if (
                detection.request_id in expected_ids
                and detection.request_id not in detections_by_request_id
            ):
                detections_by_request_id[detection.request_id] = detection

    source_by_image_index = {
        source.image_index: source for source in source_images
    }
    assets: List[Dict[str, Any]] = []
    for request in requests:
        detection = detections_by_request_id.get(request.request_id)
        image_index = detection.image_index if detection is not None else None
        if isinstance(image_index, bool):
            image_index = None
        box = _normalize_box(detection.box_2d) if detection is not None else None
        label = detection.label if detection is not None else None

        data_url = None
        status = "missing"
        if isinstance(image_index, int) and box is not None:
            source = source_by_image_index.get(image_index)
            if source is not None:
                data_url = _crop_box_to_data_url(source.image, box)
                status = "ok" if data_url else "error"

        assets.append(
            {
                "description": request.description,
                "data_url": data_url,
                "status": status,
                "box_2d": box,
                "image_index": image_index,
                "label": label,
            }
        )

    result: Dict[str, Any] = {"assets": assets}
    if any(asset["status"] != "ok" for asset in assets):
        result["error"] = (
            "Gemini 未能为所有请求素材返回可用的边界框。"
        )

    return result
