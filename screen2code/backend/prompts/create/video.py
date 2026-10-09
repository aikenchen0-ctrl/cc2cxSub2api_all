from openai.types.chat import ChatCompletionContentPartParam, ChatCompletionMessageParam
from prompts.prompt_types import Stack
from prompts import system_prompt
from prompts.design_system import build_design_system_prompt_block
from prompts.policies import build_selected_stack_policy, build_user_image_policy


def build_video_prompt_messages(
    frame_data_urls: list[str],
    stack: Stack,
    text_prompt: str,
    image_generation_enabled: bool,
    design_system: str | None = None,
) -> list[ChatCompletionMessageParam]:
    image_policy = build_user_image_policy(image_generation_enabled)
    selected_stack = build_selected_stack_policy(stack)
    design_system_block = build_design_system_prompt_block(design_system)
    user_text = f"""
    你收到的是用户操作网页的录屏采样帧，已按时间先后排序。请复刻同一应用，使相同操作得到相同的页面状态和结果。

    - 对比连续帧，理解导航、按钮点击、表单输入和界面状态变化。
    - 精确还原画面中的布局、文字、背景色、字体、字号、内外边距和边框。
    - {image_policy}
    - 需要后端数据时使用模拟数据，勿虚构真实后端接口。
    - 使用 JavaScript 实现可操作的交互，使行为与录屏展示一致。
    - 必要时使用 SVG 和可交互的 3D 元素。

    请分析这些录屏帧并生成代码。
    
    {selected_stack}
    {design_system_block}
    """
    if text_prompt.strip():
        user_text = user_text + "\n\n补充要求：" + text_prompt

    user_content: list[ChatCompletionContentPartParam] = []
    for frame_url in frame_data_urls:
        user_content.append({
            "type": "image_url",
            "image_url": {"url": frame_url, "detail": "high"},
        })
    user_content.append({
            "type": "text",
            "text": user_text,
        })

    return [
        {
            "role": "system",
            "content": system_prompt.SYSTEM_PROMPT,
        },
        {
            "role": "user",
            "content": user_content,
        },
    ]
