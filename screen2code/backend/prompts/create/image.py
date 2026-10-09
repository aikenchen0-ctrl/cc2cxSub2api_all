from openai.types.chat import ChatCompletionContentPartParam, ChatCompletionMessageParam

from prompts.prompt_types import Stack
from prompts import system_prompt
from prompts.design_system import build_design_system_prompt_block
from prompts.policies import build_selected_stack_policy, build_user_image_policy

def build_image_prompt_messages(
    image_data_urls: list[str],
    stack: Stack,
    text_prompt: str,
    image_generation_enabled: bool,
    design_system: str | None = None,
) -> list[ChatCompletionMessageParam]:
    image_policy = build_user_image_policy(image_generation_enabled)
    selected_stack = build_selected_stack_policy(stack)
    design_system_block = build_design_system_prompt_block(design_system)
    user_prompt = f"""
请生成与所提供截图外观一致的网页代码。

{selected_stack}
{design_system_block}

## 复刻要求

- 精确还原截图的布局、颜色、尺寸和间距。
- 保留截图中的原始文字。
- 尽量提取原图的准确素材，无法提取的素材可通过图片生成补充。
- 使用 extract_assets 提取素材，并仔细检查结果是否符合要求。
- edit_images 可用时，用它编辑素材或去除不需要的元素，将独立编辑合并调用。
- 素材分辨率过低、需要放大时，使用 edit_images 提升分辨率，不要靠 CSS 拉伸或 generate_images 重绘。
- 原图素材被遮挡或属于背景、无法提取时，可用 generate_images 生成图片地址并批量传入提示词。

- {image_policy}

## 多张截图

提供多张截图时，按其内容合理组织：

- 同一网站的不同页面应实现为独立页面并通过链接连接。
- 同一应用的不同标签页或视图应通过导航切换。
- 无关联的截图可通过“截图 1”“截图 2”等入口分别展示，便于浏览。
- 移动端截图只复刻实际界面，不包含设备边框和浏览器外框。
"""

    # Add additional instructions provided by the user
    if text_prompt.strip():
        user_prompt = f"{user_prompt}\n\n补充要求：{text_prompt}"

    user_content: list[ChatCompletionContentPartParam] = []
    for image_data_url in image_data_urls:
        user_content.append(
            {
                "type": "image_url",
                "image_url": {"url": image_data_url, "detail": "high"},
            }
        )
    user_content.append(
        {
            "type": "text",
            "text": user_prompt,
        }
    )
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
