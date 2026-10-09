from custom_types import InputMode
from prompts.create.image import build_image_prompt_messages
from prompts.create.text import build_text_prompt_messages
from prompts.create.video import build_video_prompt_messages
from prompts.prompt_types import Stack, UserTurnInput
from prompts.message_builder import Prompt


def build_create_prompt_from_input(
    input_mode: InputMode,
    stack: Stack,
    prompt: UserTurnInput,
    image_generation_enabled: bool,
    design_system: str | None = None,
) -> Prompt:
    if input_mode == "image":
        image_urls = prompt.get("images", [])
        text_prompt = prompt.get("text", "")
        return build_image_prompt_messages(
            image_data_urls=image_urls,
            stack=stack,
            text_prompt=text_prompt,
            image_generation_enabled=image_generation_enabled,
            design_system=design_system,
        )
    if input_mode == "text":
        return build_text_prompt_messages(
            text_prompt=prompt["text"],
            stack=stack,
            image_generation_enabled=image_generation_enabled,
            design_system=design_system,
        )
    if input_mode == "video":
        frame_urls = prompt.get("images", [])
        if not frame_urls:
            raise ValueError("视频模式需要上传视频或录制屏幕。")
        return build_video_prompt_messages(
            frame_data_urls=frame_urls,
            stack=stack,
            text_prompt=prompt.get("text", ""),
            image_generation_enabled=image_generation_enabled,
            design_system=design_system,
        )
    raise ValueError(f"不支持的输入模式： {input_mode}")


__all__ = ["build_create_prompt_from_input"]
