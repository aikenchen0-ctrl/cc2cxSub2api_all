from copy import deepcopy

from prompts.prompt_types import PromptHistoryMessage, UserTurnInput
from uploaded_assets.store import persist_data_url_as_temporary_asset


def _build_asset_id_block(images: list[str], asset_base_url: str) -> str:
    asset_lines: list[str] = []
    for index, image in enumerate(images, start=1):
        asset = persist_data_url_as_temporary_asset(image, asset_base_url)
        if asset:
            asset_lines.append(f"- 图片 {index}：asset_id `{asset.asset_id}`")

    if not asset_lines:
        return ""

    joined_asset_lines = "\n".join(asset_lines)
    return f"""

## 已上传图片素材 ID

本轮上传的图片可通过以下不透明素材 ID 供 save_assets 工具使用：

{joined_asset_lines}

部分上传图片可能是参考截图，其他图片可能是徽标、产品照片、图标、背景或插画等素材。请判断哪些上传图片应作为生成代码中的素材。

如果决定使用一张或多张上传图片作为素材，请在写入或编辑代码前将素材 ID 放入 asset_ids 列表调用 save_assets，再在代码中使用返回的永久 public_url。最终代码不得嵌入素材 ID、临时路径或临时 URL。除非用户明确要求，不要把截图或参考界面图当作页面素材。如果使用了上传素材，请在最终消息中简要说明。
"""


def append_uploaded_asset_ids_to_prompt(
    prompt: UserTurnInput,
    asset_base_url: str,
) -> UserTurnInput:
    prompt_with_assets = deepcopy(prompt)
    block = _build_asset_id_block(prompt_with_assets.get("images", []), asset_base_url)
    if block:
        prompt_with_assets["text"] = f"{prompt_with_assets.get('text', '')}\n\n{block}".strip()
    return prompt_with_assets


def append_uploaded_asset_ids_to_history(
    history: list[PromptHistoryMessage],
    asset_base_url: str,
) -> list[PromptHistoryMessage]:
    history_with_assets = deepcopy(history)
    for item in history_with_assets:
        if item["role"] != "user":
            continue
        block = _build_asset_id_block(item.get("images", []), asset_base_url)
        if block:
            item["text"] = f"{item.get('text', '')}\n\n{block}".strip()
    return history_with_assets
