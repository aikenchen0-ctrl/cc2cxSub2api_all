from __future__ import annotations

import json
from typing import Any

import httpx

from .config import Settings


class Sub2APIError(RuntimeError):
    def __init__(self, status_code: int | None, code: str):
        super().__init__(code)
        self.status_code = status_code
        self.code = code


def _payload_data(payload: Any) -> Any:
    if isinstance(payload, dict) and "data" in payload:
        return payload["data"]
    return payload


def _items_and_total(data: Any) -> tuple[list[dict[str, Any]], int | None]:
    if isinstance(data, list):
        return [item for item in data if isinstance(item, dict)], None
    if isinstance(data, dict):
        items = data.get("items")
        if isinstance(items, list):
            total = data.get("total")
            return [item for item in items if isinstance(item, dict)], int(total) if total is not None else None
        accounts = data.get("accounts")
        if isinstance(accounts, list):
            return [item for item in accounts if isinstance(item, dict)], None
    return [], None


class Sub2APIClient:
    def __init__(self, settings: Settings):
        self.settings = settings
        timeout = httpx.Timeout(settings.request_timeout_seconds, connect=15.0)
        admin_headers = {"Accept": "application/json"}
        if settings.admin_api_key:
            admin_headers["x-api-key"] = settings.admin_api_key
        elif settings.admin_jwt:
            admin_headers["Authorization"] = f"Bearer {settings.admin_jwt}"
        self.admin = httpx.AsyncClient(
            base_url=settings.admin_base_url,
            timeout=timeout,
            headers=admin_headers,
            follow_redirects=False,
        )
        self.gateway = httpx.AsyncClient(
            base_url=settings.gateway_base_url,
            timeout=timeout,
            headers={"Accept": "application/json", "Content-Type": "application/json"},
            follow_redirects=False,
        )

    async def close(self) -> None:
        await self.admin.aclose()
        await self.gateway.aclose()

    async def _admin_json(self, method: str, path: str, **kwargs: Any) -> Any:
        try:
            response = await self.admin.request(method, path, **kwargs)
        except httpx.TimeoutException as exc:
            raise Sub2APIError(None, "admin_api_timeout") from exc
        except httpx.HTTPError as exc:
            raise Sub2APIError(None, "admin_api_unreachable") from exc
        if response.status_code < 200 or response.status_code >= 300:
            raise Sub2APIError(response.status_code, f"admin_api_http_{response.status_code}")
        try:
            return response.json()
        except (json.JSONDecodeError, ValueError) as exc:
            raise Sub2APIError(response.status_code, "admin_api_invalid_json") from exc

    async def list_active_accounts(self) -> list[dict[str, Any]]:
        page = 1
        page_size = 100
        all_items: list[dict[str, Any]] = []
        while page <= 100:
            payload = await self._admin_json(
                "GET",
                "/api/v1/admin/accounts",
                params={"status": "active", "page": page, "page_size": page_size},
            )
            data = _payload_data(payload)
            items, total = _items_and_total(data)
            all_items.extend(item for item in items if item.get("status") == "active")
            if not items or (total is not None and len(all_items) >= total) or len(items) < page_size:
                break
            page += 1
        else:
            raise Sub2APIError(None, "account_list_page_limit")
        return all_items

    async def get_account(self, account_id: int) -> dict[str, Any]:
        payload = await self._admin_json("GET", f"/api/v1/admin/accounts/{account_id}")
        data = _payload_data(payload)
        if not isinstance(data, dict):
            raise Sub2APIError(None, "account_detail_invalid")
        return data

    async def get_account_usage(self, account_id: int) -> dict[str, Any]:
        payload = await self._admin_json(
            "GET",
            f"/api/v1/admin/accounts/{account_id}/usage",
            params={"source": "active", "force": "true"},
        )
        data = _payload_data(payload)
        if not isinstance(data, dict):
            raise Sub2APIError(None, "account_usage_invalid")
        return data

    async def get_user(self, user_id: str) -> dict[str, Any]:
        if not user_id.isdecimal():
            raise Sub2APIError(None, "sso_subject_invalid")
        payload = await self._admin_json("GET", f"/api/v1/admin/users/{int(user_id)}")
        data = _payload_data(payload)
        if not isinstance(data, dict):
            raise Sub2APIError(None, "admin_user_invalid")
        return data

    async def gateway_chat(
        self,
        *,
        key: str,
        model: str,
        session_id: str,
        request_id: str,
        metadata_user_id: str,
        user_agent: str | None,
        messages: list[dict[str, Any]],
        max_tokens: int,
        extra_body: dict[str, Any] | None = None,
        serialized_body: bytes | None = None,
    ) -> tuple[int, dict[str, Any] | None, str]:
        return await self._gateway_messages_request(
            "chat/completions",
            key=key,
            model=model,
            session_id=session_id,
            request_id=request_id,
            metadata_user_id=metadata_user_id,
            user_agent=user_agent,
            messages=messages,
            max_tokens=max_tokens,
            extra_body=extra_body,
            serialized_body=serialized_body,
        )

    async def gateway_messages(
        self,
        *,
        key: str,
        model: str,
        session_id: str,
        request_id: str,
        metadata_user_id: str,
        user_agent: str | None,
        messages: list[dict[str, Any]],
        max_tokens: int,
        extra_body: dict[str, Any] | None = None,
        serialized_body: bytes | None = None,
    ) -> tuple[int, dict[str, Any] | None, str]:
        return await self._gateway_messages_request(
            "messages",
            key=key,
            model=model,
            session_id=session_id,
            request_id=request_id,
            metadata_user_id=metadata_user_id,
            user_agent=user_agent,
            messages=messages,
            max_tokens=max_tokens,
            extra_body=extra_body,
            serialized_body=serialized_body,
        )

    async def _gateway_messages_request(
        self,
        path: str,
        *,
        key: str,
        model: str,
        session_id: str,
        request_id: str,
        metadata_user_id: str,
        user_agent: str | None,
        messages: list[dict[str, Any]],
        max_tokens: int,
        extra_body: dict[str, Any] | None = None,
        serialized_body: bytes | None = None,
    ) -> tuple[int, dict[str, Any] | None, str]:
        body: dict[str, Any] = {
            "model": model,
            "messages": messages,
            "max_tokens": max_tokens,
            "stream": False,
        }
        if extra_body:
            body.update(extra_body)
        metadata = body.get("metadata")
        if not isinstance(metadata, dict):
            metadata = {}
        metadata["user_id"] = metadata_user_id
        body["metadata"] = metadata
        headers = {
            "Authorization": f"Bearer {key}",
            "X-Session-Id": session_id,
            "X-Request-ID": request_id,
        }
        if path == "messages":
            headers["anthropic-version"] = "2023-06-01"
        if user_agent:
            headers["User-Agent"] = user_agent
        try:
            if serialized_body is None:
                response = await self.gateway.post(path, headers=headers, json=body)
            else:
                response = await self.gateway.post(path, headers=headers, content=serialized_body)
        except httpx.TimeoutException:
            return 0, None, "gateway_timeout"
        except httpx.HTTPError:
            return 0, None, "gateway_unreachable"
        if response.status_code < 200 or response.status_code >= 300:
            return response.status_code, None, f"gateway_http_{response.status_code}"
        try:
            payload = response.json()
            if not isinstance(payload, dict):
                payload = None
        except (json.JSONDecodeError, ValueError):
            payload = None
        return response.status_code, payload, ""

    async def set_schedulable(self, account_id: int, schedulable: bool) -> dict[str, Any]:
        payload = await self._admin_json(
            "POST",
            f"/api/v1/admin/accounts/{account_id}/schedulable",
            json={"schedulable": schedulable},
        )
        data = _payload_data(payload)
        return data if isinstance(data, dict) else {}
