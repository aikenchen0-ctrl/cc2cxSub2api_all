import base64
import subprocess
from pathlib import Path

import pytest
from imageio_ffmpeg import get_ffmpeg_exe

from agent.providers.openai import _convert_message_to_responses_input
from prompts.pipeline import build_prompt_messages
from video.frames import extract_video_frames, normalize_video_inputs


@pytest.fixture
def recording(tmp_path: Path) -> str:
    target = tmp_path / "recording.webm"
    subprocess.run(
        [get_ffmpeg_exe(), "-nostdin", "-loglevel", "error", "-f", "lavfi",
         "-i", "testsrc=size=160x100:rate=10", "-t", "1.5", "-c:v", "libvpx-vp9", str(target)],
        check=True, timeout=20,
    )
    return "data:video/webm;base64," + base64.b64encode(target.read_bytes()).decode()


@pytest.mark.asyncio
async def test_recording_becomes_valid_image_input(recording: str) -> None:
    messages = await build_prompt_messages(
        stack="html_tailwind", input_mode="video", generation_type="create",
        prompt={"text": "复刻按钮交互", "images": [], "videos": [recording]}, history=[],
    )
    content = messages[1].get("content")
    assert isinstance(content, list)
    urls = [part["image_url"]["url"] for part in content if part["type"] == "image_url"]
    assert 1 <= len(urls) <= 16
    for url in urls:
        assert url.startswith("data:image/jpeg;base64,")
        assert base64.b64decode(url.split(",", 1)[1], validate=True).startswith(b"\xff\xd8")
    assert "同一段" in str(content[-1])
    assert "data:video/" not in str([_convert_message_to_responses_input(message) for message in messages])


@pytest.mark.asyncio
async def test_history_and_selected_element_text_are_preserved(recording: str) -> None:
    prompt, history = await normalize_video_inputs(
        {"text": "修改按钮", "full_text": "仅修改选中按钮", "images": [], "videos": [recording]},
        [{"role": "user", "text": "原始录屏", "images": [], "videos": [recording]}],
    )
    assert prompt["videos"] == history[0]["videos"] == []
    assert prompt["images"] == history[0]["images"]
    assert prompt.get("full_text", "").startswith("仅修改选中按钮")
    assert "采样" in history[0]["text"]


@pytest.mark.parametrize("data_url", ["data:image/png;base64,YQ==", "data:video/webm;base64,!!!", "data:video/webm;base64,"])
def test_rejects_invalid_recordings(data_url: str) -> None:
    with pytest.raises(ValueError):
        extract_video_frames(data_url)
