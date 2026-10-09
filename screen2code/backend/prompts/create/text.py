from openai.types.chat import ChatCompletionMessageParam

from prompts.prompt_types import Stack
from prompts import system_prompt
from prompts.design_system import build_design_system_prompt_block
from prompts.policies import build_selected_stack_policy, build_user_image_policy


def build_text_prompt_messages(
    text_prompt: str,
    stack: Stack,
    image_generation_enabled: bool,
    design_system: str | None = None,
) -> list[ChatCompletionMessageParam]:
    image_policy = build_user_image_policy(image_generation_enabled)
    selected_stack = build_selected_stack_policy(stack)
    design_system_block = build_design_system_prompt_block(design_system)

    USER_PROMPT = f"""
请根据以下要求生成界面：{text_prompt}。
{selected_stack}
{design_system_block}

# 实现要求

- 页面外观应现代、简洁。
- 使用专业的字体和配色，保证中文排版清晰。
- 遵循良好的用户体验规范，界面文案默认使用简体中文。
- {image_policy}"""

    return [
        {
            "role": "system",
            "content": system_prompt.SYSTEM_PROMPT,
        },
        {
            "role": "user",
            "content": USER_PROMPT,
        },
    ]
