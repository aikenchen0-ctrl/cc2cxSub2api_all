# pyright: reportUnknownVariableType=false
import asyncio
import difflib
from typing import Any, Dict, List, Optional, Tuple, Union, cast

from codegen.utils import extract_html_content
from config import REPLICATE_API_KEY
from agent.tools.extract_assets import run_extract_assets
from agent.tools.local_assets import guess_image_mime, local_asset_url_to_data_url
from agent.tools.screenshot_preview import run_screenshot_preview
from image_generation.generation import process_tasks
from image_generation.replicate import (
    P_IMAGE_EDIT_ASPECT_RATIOS,
    PImageEditAspectRatio,
    edit_image as edit_image_once,
    remove_background as remove_background_once,
)
from uploaded_assets.tools import run_save_assets

from agent.state import AgentFileState, ensure_str
from agent.tools.types import ToolCall, ToolExecutionResult, ToolMultimodalPart
from agent.tools.summaries import summarize_text


IMAGE_TOOL_BATCH_SIZE = 20


class AgentToolRuntime:
    def __init__(
        self,
        file_state: AgentFileState,
        should_generate_images: bool,
        openai_api_key: Optional[str],
        openai_base_url: Optional[str],
        gemini_api_key: Optional[str] = None,
        replicate_api_key: Optional[str] = None,
        input_images: Optional[List[str]] = None,
        asset_base_url: str = "",
        user_id: Optional[str] = None,
        option_codes: Optional[List[str]] = None,
    ):
        self.file_state = file_state
        self.should_generate_images = should_generate_images
        self.openai_api_key = openai_api_key
        self.openai_base_url = openai_base_url
        self.gemini_api_key = gemini_api_key
        self.replicate_api_key = replicate_api_key
        self.input_images = input_images or []
        self.asset_base_url = asset_base_url
        self.user_id = user_id
        self.option_codes = option_codes or []

    def _effective_replicate_api_key(self) -> str | None:
        return self.replicate_api_key or REPLICATE_API_KEY

    async def execute(self, tool_call: ToolCall) -> ToolExecutionResult:
        if "INVALID_JSON" in tool_call.arguments:
            invalid_json = ensure_str(tool_call.arguments.get("INVALID_JSON"))
            return ToolExecutionResult(
                ok=False,
                result={
                    "error": "工具参数不是有效的 JSON。",
                    "INVALID_JSON": invalid_json,
                },
                summary={"error": "JSON 工具参数无效"},
            )

        if tool_call.name == "create_file":
            return self._create_file(tool_call.arguments)
        if tool_call.name == "edit_file":
            return self._edit_file(tool_call.arguments)
        if tool_call.name == "generate_images":
            return await self._generate_images(tool_call.arguments)
        if tool_call.name == "remove_backgrounds":
            return await self._remove_backgrounds(tool_call.arguments)
        if tool_call.name == "edit_images":
            return await self._edit_images(tool_call.arguments)
        if tool_call.name == "extract_assets":
            return await run_extract_assets(
                tool_call.arguments,
                gemini_api_key=self.gemini_api_key,
                input_images=self.input_images,
                asset_base_url=self.asset_base_url,
                user_id=self.user_id,
            )
        if tool_call.name == "screenshot_preview":
            return await run_screenshot_preview(
                tool_call.arguments,
                file_state=self.file_state,
            )
        if tool_call.name == "save_assets":
            return await run_save_assets(tool_call.arguments, user_id=self.user_id)
        if tool_call.name == "retrieve_option":
            return self._retrieve_option(tool_call.arguments)
        return ToolExecutionResult(
            ok=False,
            result={"error": f"未知工具： {tool_call.name}"},
            summary={"error": f"未知工具： {tool_call.name}"},
        )

    def _create_file(self, args: Dict[str, Any]) -> ToolExecutionResult:
        path = ensure_str(args.get("path") or self.file_state.path or "index.html")
        content = ensure_str(args.get("content"))
        if not content:
            return ToolExecutionResult(
                ok=False,
                result={"error": "create_file 需要非空的 content"},
                summary={"error": "缺少 content"},
            )

        extracted = extract_html_content(content)
        self.file_state.path = path
        self.file_state.content = extracted or content

        summary = {
            "path": self.file_state.path,
            "contentLength": len(self.file_state.content),
            "preview": summarize_text(self.file_state.content, 320),
        }
        result = {
            "content": f"已成功创建文件 {self.file_state.path}。",
            "details": {
                "path": self.file_state.path,
                "contentLength": len(self.file_state.content),
            },
        }
        return ToolExecutionResult(
            ok=True,
            result=result,
            summary=summary,
            updated_content=self.file_state.content,
        )

    @staticmethod
    def _generate_diff(old_content: str, new_content: str, path: str) -> Dict[str, Any]:
        """Generate a unified diff between old and new content."""
        old_lines = old_content.splitlines(keepends=True)
        new_lines = new_content.splitlines(keepends=True)
        diff_lines = list(
            difflib.unified_diff(old_lines, new_lines, fromfile=path, tofile=path)
        )
        diff_str = "".join(diff_lines)

        first_changed_line: Optional[int] = None
        for line in diff_lines:
            if not line.startswith("@@"):
                continue
            try:
                plus_part = line.split("+")[1].split("@@")[0].strip()
                first_changed_line = int(plus_part.split(",")[0])
            except (IndexError, ValueError):
                pass
            break

        return {
            "diff": diff_str,
            "firstChangedLine": first_changed_line,
        }

    def _apply_single_edit(
        self,
        content: str,
        old_text: str,
        new_text: str,
        count: Optional[int],
    ) -> Tuple[str, int]:
        if old_text not in content:
            return content, 0

        if count is None:
            replace_count = 1
        elif count < 0:
            replace_count = content.count(old_text)
        else:
            replace_count = count

        updated = content.replace(old_text, new_text, replace_count)
        return updated, min(replace_count, content.count(old_text))

    def _edit_file(self, args: Dict[str, Any]) -> ToolExecutionResult:
        if not self.file_state.content:
            return ToolExecutionResult(
                ok=False,
                result={"error": "尚无文件，请先调用 create_file。"},
                summary={"error": "没有可编辑的文件"},
            )

        edits = args.get("edits")
        if not edits:
            old_text = ensure_str(args.get("old_text"))
            new_text = ensure_str(args.get("new_text"))
            count = args.get("count")
            edits = [{"old_text": old_text, "new_text": new_text, "count": count}]

        if not isinstance(edits, list):
            return ToolExecutionResult(
                ok=False,
                result={"error": "edits 必须为列表"},
                summary={"error": "编辑请求数据无效"},
            )

        content = self.file_state.content
        original_content = content
        summary_edits: List[Dict[str, Any]] = []
        for edit in edits:
            old_text = ensure_str(edit.get("old_text"))
            new_text = ensure_str(edit.get("new_text"))
            count = edit.get("count")
            if not old_text:
                return ToolExecutionResult(
                    ok=False,
                    result={"error": "edit_file 需要 old_text"},
                    summary={"error": "缺少 old_text"},
                )

            content, replaced = self._apply_single_edit(content, old_text, new_text, count)
            if replaced == 0:
                return ToolExecutionResult(
                    ok=False,
                    result={"error": "未找到 old_text", "old_text": old_text},
                    summary={
                        "error": "未找到 old_text",
                        "old_text": summarize_text(old_text, 160),
                    },
                )

            summary_edits.append(
                {
                    "old_text": summarize_text(old_text, 140),
                    "new_text": summarize_text(new_text, 140),
                    "replaced": replaced,
                }
            )

        self.file_state.content = content
        path = self.file_state.path or "index.html"
        diff_info = self._generate_diff(original_content, content, path)
        summary = {
            "path": path,
            "edits": summary_edits,
            "contentLength": len(self.file_state.content),
            "diff": diff_info["diff"],
            "firstChangedLine": diff_info["firstChangedLine"],
        }
        result = {
            "content": f"已成功编辑文件 {path}。",
            "details": {
                "diff": diff_info["diff"],
                "firstChangedLine": diff_info["firstChangedLine"],
            },
        }
        return ToolExecutionResult(
            ok=True,
            result=result,
            summary=summary,
            updated_content=self.file_state.content,
        )

    async def _generate_images(self, args: Dict[str, Any]) -> ToolExecutionResult:
        if not self.should_generate_images:
            return ToolExecutionResult(
                ok=False,
                result={"error": "图片生成已关闭。"},
                summary={"error": "图片生成已关闭"},
            )

        prompts = args.get("prompts") or []
        if not isinstance(prompts, list) or not prompts:
            return ToolExecutionResult(
                ok=False,
                result={"error": "generate_images 需要非空的 prompts 列表"},
                summary={"error": "缺少 prompts"},
            )

        cleaned = [prompt.strip() for prompt in prompts if isinstance(prompt, str)]
        unique_prompts = list(dict.fromkeys([p for p in cleaned if p]))
        if not unique_prompts:
            return ToolExecutionResult(
                ok=False,
                result={"error": "未提供有效的提示词"},
                summary={"error": "没有有效的提示词"},
            )
        replicate_api_key = self._effective_replicate_api_key()
        if not replicate_api_key:
            return ToolExecutionResult(
                ok=False,
                result={"error": "没有可用于图片生成的接口密钥。"},
                summary={"error": "缺少图片生成接口密钥"},
            )
        model = "flux"
        api_key = replicate_api_key
        base_url = None

        generated = await process_tasks(unique_prompts, api_key, base_url, model)
        merged_results = {
            prompt: url for prompt, url in zip(unique_prompts, generated)
        }
        summary_items = [
            {
                "prompt": prompt,
                "url": url,
                "status": "ok" if url else "error",
            }
            for prompt, url in merged_results.items()
        ]
        result = {"images": merged_results}
        summary = {"images": summary_items}
        multimodal_parts = [
            ToolMultimodalPart(
                display_name=f"generated_{index}.png",
                mime_type=guess_image_mime(url),
                image_url=url,
            )
            for index, url in enumerate(merged_results.values())
            if url
        ]
        return ToolExecutionResult(
            ok=True,
            result=result,
            summary=summary,
            multimodal_parts=multimodal_parts,
        )

    async def _remove_backgrounds(
        self, args: Dict[str, Any]
    ) -> ToolExecutionResult:
        replicate_api_key = self._effective_replicate_api_key()
        if not replicate_api_key:
            return ToolExecutionResult(
                ok=False,
                result={"error": "移除背景需要配置 REPLICATE_API_KEY。"},
                summary={"error": "缺少 Replicate 接口密钥"},
            )

        image_urls = args.get("image_urls") or []
        if not isinstance(image_urls, list) or not image_urls:
            return ToolExecutionResult(
                ok=False,
                result={
                    "error": "remove_backgrounds 需要非空的 image_urls 列表"
                },
                summary={"error": "缺少 image_urls"},
            )

        cleaned = [url.strip() for url in image_urls if isinstance(url, str)]
        unique_urls = list(dict.fromkeys([u for u in cleaned if u]))
        if not unique_urls:
            return ToolExecutionResult(
                ok=False,
                result={"error": "未提供有效的图片 URL"},
                summary={"error": "没有有效的 image_urls"},
            )

        raw_results: list[str | BaseException] = []
        for i in range(0, len(unique_urls), IMAGE_TOOL_BATCH_SIZE):
            batch = unique_urls[i : i + IMAGE_TOOL_BATCH_SIZE]
            # Replicate can't fetch localhost; inline local assets as data URLs.
            tasks = [
                remove_background_once(
                    local_asset_url_to_data_url(url), replicate_api_key
                )
                for url in batch
            ]
            raw_results.extend(await asyncio.gather(*tasks, return_exceptions=True))

        results: List[Dict[str, Any]] = []
        for url, raw in zip(unique_urls, raw_results):
            if isinstance(raw, BaseException):
                print(f"Background removal failed for {url}: {raw}")
                results.append(
                    {"image_url": url, "result_url": None, "status": "error"}
                )
            else:
                results.append(
                    {"image_url": url, "result_url": raw, "status": "ok"}
                )

        summary_items = [
            {
                "image_url": summarize_text(r["image_url"], 100),
                "result_url": r["result_url"],
                "status": r["status"],
            }
            for r in results
        ]
        multimodal_parts = [
            ToolMultimodalPart(
                display_name=f"no_bg_{index}.png",
                mime_type=guess_image_mime(result["result_url"]),
                image_url=result["result_url"],
            )
            for index, result in enumerate(results)
            if result["status"] == "ok" and result["result_url"]
        ]
        return ToolExecutionResult(
            ok=True,
            result={"images": results},
            summary={"images": summary_items},
            multimodal_parts=multimodal_parts,
        )

    async def _edit_images(self, args: Dict[str, Any]) -> ToolExecutionResult:
        replicate_api_key = self._effective_replicate_api_key()
        if not replicate_api_key:
            return ToolExecutionResult(
                ok=False,
                result={"error": "图片编辑需要配置 REPLICATE_API_KEY。"},
                summary={"error": "缺少 Replicate 接口密钥"},
            )

        raw_edits = args.get("edits") or []
        if not isinstance(raw_edits, list) or not raw_edits:
            return ToolExecutionResult(
                ok=False,
                result={"error": "edit_images 需要非空的 edits 列表"},
                summary={"error": "缺少 edits"},
            )

        results: List[Dict[str, Any]] = []
        valid_indexes: List[int] = []
        for index, raw_edit in enumerate(cast(List[object], raw_edits)):
            if not isinstance(raw_edit, dict):
                results.append(
                    {
                        "prompt": "",
                        "image_urls": [],
                        "result_url": None,
                        "status": "error",
                        "aspect_ratio": "match_input_image",
                        "error": f"编辑项 {index + 1} 必须为对象。",
                    }
                )
                continue

            edit = cast(Dict[str, Any], raw_edit)
            prompt = ensure_str(edit.get("prompt")).strip()
            raw_image_urls = edit.get("image_urls") or []
            image_urls = (
                [
                    url.strip()
                    for url in cast(List[object], raw_image_urls)
                    if isinstance(url, str) and url.strip()
                ]
                if isinstance(raw_image_urls, list)
                else []
            )
            aspect_ratio_value = ensure_str(
                edit.get("aspect_ratio") or "match_input_image"
            )
            if aspect_ratio_value not in P_IMAGE_EDIT_ASPECT_RATIOS:
                aspect_ratio_value = "match_input_image"

            item: Dict[str, Any] = {
                "prompt": prompt,
                "image_urls": image_urls,
                "result_url": None,
                "status": "pending",
                "aspect_ratio": aspect_ratio_value,
            }
            errors: List[str] = []
            if not prompt:
                errors.append("prompt 不能为空")
            if not image_urls:
                errors.append("image_urls 必须包含至少一个有效 URL")
            if errors:
                item["status"] = "error"
                item["error"] = "; ".join(errors)
            else:
                valid_indexes.append(index)
            results.append(item)

        async def execute_single_edit(item: Dict[str, Any]) -> str:
            image_urls = cast(List[str], item["image_urls"])
            return await edit_image_once(
                prompt=ensure_str(item["prompt"]),
                image_urls=[local_asset_url_to_data_url(url) for url in image_urls],
                api_token=replicate_api_key,
                aspect_ratio=cast(PImageEditAspectRatio, item["aspect_ratio"]),
            )

        for i in range(0, len(valid_indexes), IMAGE_TOOL_BATCH_SIZE):
            batch_indexes = valid_indexes[i : i + IMAGE_TOOL_BATCH_SIZE]
            raw_results = await asyncio.gather(
                *(execute_single_edit(results[index]) for index in batch_indexes),
                return_exceptions=True,
            )
            for index, raw in zip(batch_indexes, raw_results):
                item = results[index]
                if isinstance(raw, BaseException):
                    print(f"Image edit failed for {item['image_urls']}: {raw}")
                    item["status"] = "error"
                    item["error"] = str(raw)
                else:
                    item["result_url"] = raw
                    item["status"] = "ok"

        summary_items = [
            {
                "prompt": item["prompt"],
                "image_urls": cast(List[str], item["image_urls"]),
                "result_url": item["result_url"],
                "status": item["status"],
                "aspect_ratio": item["aspect_ratio"],
                **({"error": item["error"]} if item.get("error") else {}),
            }
            for item in results
        ]
        multimodal_parts = [
            ToolMultimodalPart(
                display_name=f"edited_{index}.png",
                mime_type=guess_image_mime(item["result_url"]),
                image_url=item["result_url"],
            )
            for index, item in enumerate(results)
            if item["status"] == "ok" and item["result_url"]
        ]
        return ToolExecutionResult(
            ok=True,
            result={"images": results},
            summary={"images": summary_items},
            multimodal_parts=multimodal_parts,
        )

    def _retrieve_option(self, args: Dict[str, Any]) -> ToolExecutionResult:
        raw_option_number = args.get("option_number")
        raw_index = args.get("index")

        def coerce_int(value: Any) -> Optional[int]:
            if value is None:
                return None
            try:
                return int(value)
            except (TypeError, ValueError):
                return None

        option_number = coerce_int(raw_option_number)
        index = coerce_int(raw_index)

        if option_number is None and index is None:
            return ToolExecutionResult(
                ok=False,
                result={"error": "retrieve_option 需要 option_number"},
                summary={"error": "缺少 option_number"},
            )

        resolved_index = index if option_number is None else option_number - 1
        if resolved_index is None:
            return ToolExecutionResult(
                ok=False,
                result={"error": "option_number 无效"},
                summary={"error": "option_number 无效"},
            )

        if resolved_index < 0 or resolved_index >= len(self.option_codes):
            return ToolExecutionResult(
                ok=False,
                result={
                    "error": "方案编号超出范围",
                    "option_number": resolved_index + 1,
                    "available": len(self.option_codes),
                },
                summary={
                    "error": "方案编号超出范围",
                    "available": len(self.option_codes),
                },
            )

        code = ensure_str(self.option_codes[resolved_index])
        if not code.strip():
            return ToolExecutionResult(
                ok=False,
                result={
                    "error": "方案代码为空或不可用",
                    "option_number": resolved_index + 1,
                },
                summary={"error": "方案代码不可用"},
            )

        summary = {
            "option_number": resolved_index + 1,
            "contentLength": len(code),
            "preview": summarize_text(code, 200),
        }
        result = {"option_number": resolved_index + 1, "code": code}
        return ToolExecutionResult(ok=True, result=result, summary=summary)


# Backwards-compatible alias for older imports.
AgentToolbox = AgentToolRuntime
