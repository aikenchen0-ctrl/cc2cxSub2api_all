from prompts.prompt_types import Stack


def build_selected_stack_policy(stack: Stack) -> str:
    return f"所选技术栈：{stack}。"


def build_user_image_policy(image_generation_enabled: bool) -> str:
    if image_generation_enabled:
        return (
            "本次请求已启用图片生成。需要补充缺少的素材时使用 generate_images。"
        )

    return (
        "本次请求已关闭图片生成，不要调用 generate_images。"
        "请使用提供的素材、CSS 效果或占位图片（https://placehold.co）。"
    )
