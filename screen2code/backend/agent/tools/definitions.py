from typing import Any, Dict, List

from agent.tools.types import CanonicalToolDefinition
from image_generation.replicate import P_IMAGE_EDIT_ASPECT_RATIOS
from uploaded_assets.tools import SAVE_ASSETS_TOOL_DEFINITION


def _create_schema() -> Dict[str, Any]:
    return {
        "type": "object",
        "properties": {
            "path": {
                "type": "string",
                "description": "主 HTML 文件路径；不确定时使用 index.html。",
            },
            "content": {
                "type": "string",
                "description": "单文件应用的完整 HTML。",
            },
        },
        "required": ["content"],
    }


def _edit_schema() -> Dict[str, Any]:
    return {
        "type": "object",
        "properties": {
            "path": {
                "type": "string",
                "description": "主 HTML 文件路径。",
            },
            "old_text": {
                "type": "string",
                "description": "要替换的精确文本，必须与文件内容完全一致。",
            },
            "new_text": {
                "type": "string",
                "description": "替换后的文本。",
            },
            "count": {
                "type": "integer",
                "description": "要替换的出现次数；使用 -1 替换全部。",
            },
            "edits": {
                "type": "array",
                "items": {
                    "type": "object",
                    "properties": {
                        "old_text": {"type": "string"},
                        "new_text": {"type": "string"},
                        "count": {"type": "integer"},
                    },
                    "required": ["old_text", "new_text"],
                },
            },
        },
    }


def _image_schema() -> Dict[str, Any]:
    return {
        "type": "object",
        "properties": {
            "prompts": {
                "type": "array",
                "items": {
                    "type": "string",
                    "description": "描述一张待生成图片的提示词。",
                },
            }
        },
        "required": ["prompts"],
    }


def _remove_backgrounds_schema() -> Dict[str, Any]:
    return {
        "type": "object",
        "properties": {
            "image_urls": {
                "type": "array",
                "items": {
                    "type": "string",
                    "description": "要移除背景的图片 URL。",
                },
            },
        },
        "required": ["image_urls"],
    }


def _edit_images_schema() -> Dict[str, Any]:
    edit_properties: Dict[str, Any] = {
        "prompt": {
            "type": "string",
            "description": (
                "对本次独立编辑的清晰说明。提供多张图片时，请按图片 1、图片 2 等称呼输入。"
            ),
        },
        "image_urls": {
            "type": "array",
            "items": {
                "type": "string",
                "description": "源图片或参考图片 URL。",
            },
            "description": (
                "本次编辑按顺序排列的图片 URL：先放主图，再放参考图。"
            ),
        },
        "aspect_ratio": {
            "type": "string",
            "enum": list(P_IMAGE_EDIT_ASPECT_RATIOS),
            "default": "match_input_image",
            "description": (
                "可选的编辑图片宽高比；使用 match_input_image 可匹配主图。"
            ),
        },
    }
    return {
        "type": "object",
        "properties": {
            "edits": {
                "type": "array",
                "minItems": 1,
                "items": {
                    "type": "object",
                    "properties": edit_properties,
                    "required": ["prompt", "image_urls"],
                },
                "description": (
                    "并行执行的独立图片编辑，结果按输入顺序返回。"
                    ""
                ),
            },
        },
        "required": ["edits"],
    }


def _extract_assets_schema() -> Dict[str, Any]:
    return {
        "type": "object",
        "properties": {
            "asset_descriptions": {
                "type": "array",
                "items": {
                    "type": "string",
                    "description": (
                        "准确识别一个视觉素材的出现位置。包含独特外观（颜色、形状、内容或可见文字标识）、精确位置及附近界面/上下文；多张截图时写明从 1 开始的截图编号。重复的相似素材应分别列出并区分位置（例如最左与最右），不要只写笼统类别。"
                    ),
                },
            },
        },
        "required": ["asset_descriptions"],
    }


def _screenshot_preview_schema() -> Dict[str, Any]:
    return {
        "type": "object",
        "properties": {},
    }


def _retrieve_option_schema() -> Dict[str, Any]:
    return {
        "type": "object",
        "properties": {
            "option_number": {
                "type": "integer",
                "description": "要读取的方案编号，从 1 开始（方案 1、方案 2 等）。",
            }
        },
        "required": ["option_number"],
    }


def canonical_tool_definitions(
    image_generation_enabled: bool = True,
    image_editing_enabled: bool = True,
    asset_extraction_enabled: bool = True,
    screenshot_enabled: bool = True,
) -> List[CanonicalToolDefinition]:
    tools: List[CanonicalToolDefinition] = [
        CanonicalToolDefinition(
            name="create_file",
            description=(
                "创建应用的主 HTML 文件。必须只调用一次写入完整 HTML，并返回成功消息和文件元数据。"
            ),
            parameters=_create_schema(),
        ),
        CanonicalToolDefinition(
            name="edit_file",
            description=(
                "使用精确字符串替换编辑主 HTML 文件，不要重新生成整个文件。返回成功消息和编辑详情，包括统一 diff 与首个变更行。"
            ),
            parameters=_edit_schema(),
        ),
    ]
    if image_generation_enabled:
        tools.append(
            CanonicalToolDefinition(
                name="generate_images",
                description=(
                    "使用图片生成模型根据提示词生成图片 URL。请详细描述提示词；描述人物时写明族裔、发色、五官等外观信息。可一次传入多个提示词。"
                ),
                parameters=_image_schema(),
            )
        )
    tools.extend(
        [
            CanonicalToolDefinition(
                name="remove_backgrounds",
                description=(
                    "批量移除一张或多张图片的背景，按输入顺序返回透明背景处理图片的 URL。"
                ),
                parameters=_remove_backgrounds_schema(),
            ),
        ]
    )
    if image_editing_enabled:
        tools.append(
            CanonicalToolDefinition(
                name="edit_images",
                description=(
                    "批量执行彼此独立的图片编辑或放大。每项编辑都有自己的提示词、按顺序排列的主图/参考图 URL 和可选宽高比，结果按编辑顺序返回。"
                ),
                parameters=_edit_images_schema(),
            )
        )
    if asset_extraction_enabled:
        tools.append(
            CanonicalToolDefinition(
                name="extract_assets",
                description=(
                    "使用 Gemini 从输入截图或参考图中提取一个或多个紧密裁剪的视觉素材。每个列表项只描述一个出现位置，包含独特外观、精确位置、附近上下文以及从 1 开始的截图编号；重复相似素材要分别区分，不要只写笼统类别。按请求顺序返回带永久可嵌入 public_url 的素材和裁剪预览；确实不存在或无法单独提取的项目标记为未解析。这些素材已经保存，不要再次调用 save_assets（save_assets 仅用于用户上传图片）。"
                ),
                parameters=_extract_assets_schema(),
            )
        )
    if screenshot_enabled:
        tools.append(
            CanonicalToolDefinition(
                name="screenshot_preview",
                description=(
                    "在无头浏览器中渲染当前 HTML 文件并返回桌面端和移动端全页截图，以便目视验证。创建或大幅编辑文件后使用，检查布局、间距以及与目标设计的一致性。截图会作为附件图片返回。"
                ),
                parameters=_screenshot_preview_schema(),
            )
        )
    tools.extend(
        [
            SAVE_ASSETS_TOOL_DEFINITION,
            CanonicalToolDefinition(
                name="retrieve_option",
                description=(
                    "获取指定方案（变体）的完整 HTML 供参考。"
                ),
                parameters=_retrieve_option_schema(),
            ),
        ]
    )
    return tools
