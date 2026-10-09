"""将录屏转换为模型可理解的图片帧，不向图片字段发送原始视频。"""

import asyncio
import base64
import binascii
import math
import re
import subprocess
import tempfile
from pathlib import Path
from typing import cast

from prompts.prompt_types import PromptHistoryMessage, UserTurnInput

MAX_VIDEO_BYTES = 100 * 1024 * 1024
MAX_VIDEO_SECONDS = 180
MAX_VIDEO_FRAMES = 16


def extract_video_frames(data_url: str) -> tuple[list[str], str]:
    header, separator, encoded = data_url.partition(",")
    if not separator or not header.startswith("data:video/") or ";base64" not in header:
        raise ValueError("视频格式无效，请上传 MP4、MOV 或 WebM 文件。")
    if len(encoded) > (MAX_VIDEO_BYTES * 4 // 3 + 4):
        raise ValueError("视频超过 100MB，请缩短录制时间后重试。")
    try:
        video_bytes = base64.b64decode(encoded, validate=True)
    except (ValueError, binascii.Error) as exc:
        raise ValueError("视频数据损坏，请重新录制或上传。") from exc
    if not video_bytes or len(video_bytes) > MAX_VIDEO_BYTES:
        raise ValueError("视频为空或超过 100MB，请重新录制。")

    # imageio-ffmpeg is already installed as a MoviePy dependency.
    from imageio_ffmpeg import get_ffmpeg_exe  # type: ignore[import-untyped]

    ffmpeg = cast(str, get_ffmpeg_exe())
    with tempfile.TemporaryDirectory(prefix="screen2code-video-") as folder:
        source = Path(folder) / "recording.video"
        source.write_bytes(video_bytes)
        try:
            metadata = subprocess.run(
                [ffmpeg, "-hide_banner", "-i", str(source)],
                capture_output=True, text=True, timeout=10, check=False,
            )
            match = re.search(r"Duration: (\d+):(\d+):(\d+(?:\.\d+)?)", metadata.stderr)
            if match is None:
                raise ValueError("无法读取视频时长，请重新录制或转换为 MP4/WebM。")
            duration = int(match[1]) * 3600 + int(match[2]) * 60 + float(match[3])
            if not math.isfinite(duration) or duration <= 0:
                raise ValueError("视频时长无效，请重新录制。")
            if duration > MAX_VIDEO_SECONDS:
                raise ValueError("视频超过 3 分钟，请缩短录制时间后重试。")
            frame_count = min(MAX_VIDEO_FRAMES, max(2, math.ceil(duration * 2)))
            interval = duration / frame_count
            result = subprocess.run(
                [ffmpeg, "-hide_banner", "-loglevel", "error", "-nostdin", "-i", str(source),
                 "-vf", f"fps={1 / interval}:start_time=0,scale=1280:1280:force_original_aspect_ratio=decrease",
                 "-frames:v", str(frame_count), "-q:v", "2", str(Path(folder) / "frame-%03d.jpg")],
                capture_output=True, timeout=45, check=False,
            )
        except (subprocess.TimeoutExpired, OSError) as exc:
            raise ValueError("视频解析失败或超时，请缩短录制时间后重试。") from exc
        paths = sorted(Path(folder).glob("frame-*.jpg"))
        if result.returncode != 0 or not paths:
            raise ValueError("无法提取视频画面，请重新录制或转换为 MP4/WebM。")
        frames = ["data:image/jpeg;base64," + base64.b64encode(path.read_bytes()).decode("ascii") for path in paths]
        timeline = "、".join(f"{index * interval:.1f}秒" for index in range(len(frames)))
        description = (
            f"以下 {len(frames)} 张图片按时间顺序采样自同一段 {duration:.1f} 秒的录屏，"
            f"采样时间约为：{timeline}。请结合连续画面的变化复刻页面及交互，不要将它们当作独立页面。"
            "采样帧无法展示每一个瞬间；未明确展示的行为请依据页面状态合理实现。"
        )
        return frames, description


async def normalize_video_inputs(
    prompt: UserTurnInput, history: list[PromptHistoryMessage]
) -> tuple[UserTurnInput, list[PromptHistoryMessage]]:
    cache: dict[str, tuple[list[str], str]] = {}

    async def convert(item: UserTurnInput) -> UserTurnInput:
        videos = item.get("videos", [])
        if not videos:
            return item
        images = list(item.get("images", []))
        descriptions: list[str] = []
        for video in videos:
            if video not in cache:
                cache[video] = await asyncio.to_thread(extract_video_frames, video)
            frames, description = cache[video]
            images.extend(frames)
            descriptions.append(description)
        converted: UserTurnInput = {**item, "images": images, "videos": []}
        note = "\n\n".join(descriptions)
        converted["text"] = "\n\n".join(filter(None, [item.get("text", ""), note]))
        full_text = item.get("full_text")
        if full_text:
            converted["full_text"] = full_text + "\n\n" + note
        return converted

    normalized_prompt = await convert(prompt)
    normalized_history: list[PromptHistoryMessage] = []
    for item in history:
        if item["role"] == "user":
            normalized_history.append(cast(PromptHistoryMessage, {**await convert(cast(UserTurnInput, item)), "role": "user"}))
        else:
            normalized_history.append(item)
    return normalized_prompt, normalized_history
