"""LLM tool — unified multi-provider interface via litellm.

Supports: OpenAI, Anthropic, OpenRouter, Groq, GLM (智谱), Kimi (Moonshot),
MiniMax, and 100+ other providers through litellm.
"""

from __future__ import annotations

import asyncio
import logging
import os
from typing import Any

import litellm

from config.settings import Settings, get_settings
from tools.llm_errors import format_llm_error
from tools.llm_rate_limiter import get_llm_rate_limiter


# Suppress litellm's verbose logging by default
litellm.suppress_debug_info = True
logger = logging.getLogger(__name__)


_RESPONSE_FORMAT_UNSUPPORTED_PREFIXES = (
    "zai/",
)

_RATE_LIMIT_BACKOFF_SECONDS = (2, 5, 10)


def normalize_minimax_api_base(api_base: str) -> str:
    """Normalize MiniMax API base URLs to the OpenAI-compatible `/v1` form."""
    base = (api_base or "").strip().rstrip("/")
    if not base:
        return api_base
    if base.endswith("/anthropic"):
        logger.warning("Normalizing legacy MiniMax API base from %s to OpenAI-compatible /v1", api_base)
        return base[: -len("/anthropic")] + "/v1"
    return base


def normalize_model_name(model: str) -> str:
    """Normalize legacy provider/model aliases before sending to LiteLLM.

    MiniMax exposes an Anthropic-compatible endpoint, so older config may use
    `anthropic/MiniMax-*`. LiteLLM treats that as the real Anthropic provider
    and expects `ANTHROPIC_API_KEY`, which breaks auth when only
    `MINIMAX_API_KEY` is configured.
    """
    if model.startswith("anthropic/") and "minimax" in model.lower():
        normalized = "minimax/" + model.split("/", 1)[1]
        logger.warning("Normalizing legacy model alias from %s to %s", model, normalized)
        return normalized
    return model


def _strip_openai_routing_prefix(model: str) -> str:
    name = (model or "").strip()
    lowered = name.lower()
    if lowered.startswith("openai/"):
        name = name.split("/", 1)[1]
        lowered = name.lower()
    if lowered.startswith("responses/"):
        name = name.split("/", 1)[1]
    return name


def _is_openai_family_model(model: str) -> bool:
    name = (model or "").strip().lower()
    if not name:
        return False
    if name.startswith("openai/") or name.startswith("responses/"):
        return True
    return "/" not in name


def _is_known_openai_chat_model(model: str) -> bool:
    """True for names LiteLLM already routes as OpenAI without a prefix."""
    bare = _strip_openai_routing_prefix(model).lower()
    return bare.startswith(("gpt-", "o1", "o3", "chatgpt-"))


def _looks_like_openai_reasoning_model(model: str) -> bool:
    if not _is_openai_family_model(model):
        return False
    bare = _strip_openai_routing_prefix(model).lower()
    return bare.startswith("gpt-5") or bare.startswith("gpt-6") or bare.startswith("o1") or bare.startswith("o3")


def apply_openai_api_mode(
    model: str,
    *,
    mode: str = "auto",
    api_base: str = "",
) -> str:
    """Route OpenAI-family models through /v1/responses when the gateway needs it.

    Callers still pass chat messages. LiteLLM needs an OpenAI provider prefix,
    so we send `openai/responses/<model>` and it hits `/v1/responses`.
    """
    normalized = normalize_model_name(model)
    if not _is_openai_family_model(normalized):
        return normalized
    chosen = str(mode or "auto").strip().lower() or "auto"
    has_custom_base = bool(str(api_base or "").strip())
    use_responses = chosen == "responses" or (
        chosen == "auto" and has_custom_base and _looks_like_openai_reasoning_model(normalized)
    )
    core = _strip_openai_routing_prefix(normalized)
    if use_responses:
        return f"openai/responses/{core}"
    if has_custom_base and not _is_known_openai_chat_model(core):
        # LiteLLM only knows gpt-*/o1/o3 as OpenAI. Custom-gateway names
        # like grok-4.6 need the openai/ prefix to hit OPENAI_API_BASE.
        return f"openai/{core}"
    return core


_ALLOWED_REASONING_EFFORT = ("none", "low", "medium", "high", "xhigh", "max")


def reasoning_effort_for_model(model: str, effort: str) -> str:
    """Return a valid GPT-5.x effort, or empty when the model/value does not use it."""
    chosen = str(effort or "").strip().lower()
    if chosen not in _ALLOWED_REASONING_EFFORT:
        return ""
    if not _looks_like_openai_reasoning_model(model):
        return ""
    return chosen


def temperature_for_model(model: str, temperature: float) -> float:
    """gpt-5/gpt-6 reasoning models only accept temperature=1 via LiteLLM."""
    name = (model or "").strip().lower()
    if name.startswith("gpt-5") or name.startswith("gpt-6") or "/gpt-5" in name or "/gpt-6" in name:
        if abs(float(temperature) - 1.0) > 1e-9:
            logger.info("Forcing temperature=1 for reasoning model %s (requested %s)", model, temperature)
        return 1.0
    return temperature


def _is_retryable_transient_error(exc: Exception) -> bool:
    message = str(exc or "").lower()
    if "access forbidden" in message or "insufficient_balance" in message:
        return False
    return (
        "rate_limit" in message
        or '"http_code":"429"' in message
        or '"http_code": "429"' in message
        or "上游返回 429" in message
        or "too many requests" in message
        or "service temporarily unavailable" in message
        or "serviceunavailable" in message
        or "error code: 503" in message
        or "error code: 502" in message
        or "temporarily unavailable" in message
        or "bad gateway" in message
    )


async def acompletion_with_retry(limiter, **kwargs: Any) -> Any:
    """Call litellm.acompletion, retrying brief 429/502/503 outages."""
    last_exc: Exception | None = None
    model = kwargs.get("model", "")
    for attempt in range(len(_RATE_LIMIT_BACKOFF_SECONDS) + 1):
        try:
            await limiter.acquire()
            return await litellm.acompletion(**kwargs)
        except Exception as exc:
            last_exc = exc
            if attempt >= len(_RATE_LIMIT_BACKOFF_SECONDS) or not _is_retryable_transient_error(exc):
                raise
            delay_seconds = _RATE_LIMIT_BACKOFF_SECONDS[attempt]
            logger.warning(
                "LLM transient error for model %s; retrying in %ss (attempt %s/%s): %s",
                model,
                delay_seconds,
                attempt + 1,
                len(_RATE_LIMIT_BACKOFF_SECONDS) + 1,
                exc,
            )
            await asyncio.sleep(delay_seconds)
    if last_exc is not None:
        raise last_exc
    raise RuntimeError("LLM call failed without an exception")


def _provider_key_map(settings: Settings, scope: str) -> dict[str, str]:
    if scope in {"email", "email_reasoning"}:
        return {
            "OPENAI_API_KEY": settings.email_openai_api_key or settings.openai_api_key,
            "OPENAI_API_BASE": (settings.openai_api_base or "").strip(),
            "ANTHROPIC_API_KEY": settings.email_anthropic_api_key or settings.anthropic_api_key,
            "OPENROUTER_API_KEY": settings.email_openrouter_api_key or settings.openrouter_api_key,
            "GROQ_API_KEY": settings.email_groq_api_key or settings.groq_api_key,
            "ZAI_API_KEY": settings.email_zai_api_key or settings.zai_api_key,
            "MOONSHOT_API_KEY": settings.email_moonshot_api_key or settings.moonshot_api_key,
            "MINIMAX_API_KEY": settings.email_minimax_api_key or settings.minimax_api_key,
            "MINIMAX_API_BASE": normalize_minimax_api_base(settings.minimax_api_base),
            "ZHIPUAI_API_KEY": settings.email_zai_api_key or settings.zai_api_key,
        }
    return {
        "OPENAI_API_KEY": settings.openai_api_key,
        "OPENAI_API_BASE": (settings.openai_api_base or "").strip(),
        "ANTHROPIC_API_KEY": settings.anthropic_api_key,
        "OPENROUTER_API_KEY": settings.openrouter_api_key,
        "GROQ_API_KEY": settings.groq_api_key,
        "ZAI_API_KEY": settings.zai_api_key,
        "MOONSHOT_API_KEY": settings.moonshot_api_key,
        "MINIMAX_API_KEY": settings.minimax_api_key,
        "MINIMAX_API_BASE": normalize_minimax_api_base(settings.minimax_api_base),
        "ZHIPUAI_API_KEY": settings.zai_api_key,
    }


def _inject_api_keys(settings: Settings, scope: str = "default") -> None:
    """Push provider API keys from Settings into env vars for litellm."""
    _key_map = _provider_key_map(settings, scope)
    for env_var, value in _key_map.items():
        if value:
            os.environ[env_var] = value
        elif env_var == "OPENAI_API_BASE":
            os.environ.pop(env_var, None)

    # Native workaround: If using anthropic/ prefix for MiniMax, inject ANTHROPIC_API_BASE
    llm_models = [
        settings.llm_model,
        settings.reasoning_model,
        settings.email_llm_model,
        settings.email_reasoning_model,
    ]
    if any(model.startswith("anthropic/") and "minimax" in model.lower() for model in llm_models if model):
        os.environ["ANTHROPIC_API_BASE"] = normalize_minimax_api_base(settings.minimax_api_base)


def _select_model(settings: Settings, model_type: str) -> str:
    if model_type == "reasoning":
        return settings.reasoning_model
    if model_type == "email":
        return settings.email_llm_model or settings.llm_model
    if model_type == "email_reasoning":
        return settings.email_reasoning_model or settings.reasoning_model
    return settings.llm_model


class LLMTool:
    """Unified LLM client powered by litellm.

    All calls go through a single interface so agents don't care about the
    provider.  Just set ``llm_model`` in Settings (or ``LLM_MODEL`` in .env)
    using litellm model naming, e.g.:
      - ``gpt-4o``
      - ``anthropic/claude-3-5-sonnet-20241022``
      - ``openrouter/google/gemini-pro``
      - ``groq/llama-3.3-70b-versatile``
      - ``zai/glm-4.7``
      - ``moonshot/moonshot-v1-128k``
      - ``minimax/MiniMax-Text-01``

    Args:
        model_type: ``"default"`` uses ``llm_model`` (fast, cheap — data extraction).
                    ``"reasoning"`` uses ``reasoning_model`` (strong reasoning — ReAct decisions).
        settings: Optional Settings override.
        hunt_id: Optional hunt ID for cost tracking.
        agent: Agent name label for cost tracking (e.g. "keyword_gen", "email_craft").
        hunt_round: Current hunt round for per-round cost breakdown.
    """

    def __init__(
        self,
        model_type: str = "default",
        settings: Settings | None = None,
        hunt_id: str = "",
        agent: str = "unknown",
        hunt_round: int = 0,
    ) -> None:
        self._settings = settings or get_settings()
        self._model_type = model_type
        self._hunt_id = hunt_id
        self._agent = agent
        self._hunt_round = hunt_round
        _inject_api_keys(self._settings, self._model_type)

    @property
    def model(self) -> str:
        return apply_openai_api_mode(
            _select_model(self._settings, self._model_type),
            mode=getattr(self._settings, "openai_api_mode", "auto"),
            api_base=getattr(self._settings, "openai_api_base", ""),
        )

    @property
    def _default_temperature(self) -> float:
        if self._model_type in {"reasoning", "email_reasoning"}:
            return self._settings.reasoning_temperature
        return self._settings.llm_temperature

    @property
    def _default_max_tokens(self) -> int:
        if self._model_type in {"reasoning", "email_reasoning"}:
            return self._settings.reasoning_max_tokens
        return self._settings.llm_max_tokens

    @property
    def _requests_per_minute(self) -> int:
        if self._model_type == "reasoning":
            return self._settings.reasoning_requests_per_minute or self._settings.llm_requests_per_minute
        if self._model_type == "email":
            return self._settings.email_llm_requests_per_minute or self._settings.llm_requests_per_minute
        if self._model_type == "email_reasoning":
            return self._settings.email_reasoning_requests_per_minute or self._settings.reasoning_requests_per_minute or self._settings.llm_requests_per_minute
        return self._settings.llm_requests_per_minute

    def _supports_response_format(self) -> bool:
        """Return whether the current provider accepts OpenAI-style response_format."""
        return not self.model.startswith(_RESPONSE_FORMAT_UNSUPPORTED_PREFIXES)

    async def generate(
        self,
        prompt: str,
        *,
        system: str = "",
        temperature: float | None = None,
        max_tokens: int | None = None,
        response_format: dict | None = None,
    ) -> str:
        """Generate a completion from the LLM.

        Args:
            prompt: User message / main prompt.
            system: Optional system message.
            temperature: Override default temperature.
            max_tokens: Override default max_tokens.
            response_format: Optional JSON mode config.

        Returns:
            The generated text content.
        """
        temp = temperature_for_model(
            self.model,
            temperature if temperature is not None else self._default_temperature,
        )
        tokens = max_tokens or self._default_max_tokens

        messages: list[dict[str, str]] = []
        if system:
            messages.append({"role": "system", "content": system})
        messages.append({"role": "user", "content": prompt})

        kwargs: dict[str, Any] = {
            "model": self.model,
            "messages": messages,
            "temperature": temp,
            "max_tokens": tokens,
        }
        effort = reasoning_effort_for_model(
            self.model,
            getattr(self._settings, "llm_reasoning_effort", "") or "",
        )
        if effort:
            if "/responses/" in self.model:
                kwargs["reasoning"] = {"effort": effort}
            else:
                kwargs["reasoning_effort"] = effort
        if response_format:
            if self._supports_response_format():
                kwargs["response_format"] = response_format
            else:
                logger.info(
                    "Skipping response_format for model %s because the provider does not support it",
                    self.model,
                )

        limiter = get_llm_rate_limiter(self._model_type, self._requests_per_minute)
        try:
            response = await acompletion_with_retry(limiter, **kwargs)
        except Exception as exc:
            raise RuntimeError(format_llm_error(exc)) from exc

        # Record cost to tracker if hunt_id is set
        if self._hunt_id:
            try:
                from observability.cost_tracker import get_tracker
                usage = getattr(response, "usage", None)
                if usage:
                    cost = getattr(response, "_hidden_params", {}).get("response_cost") or 0.0
                    get_tracker(self._hunt_id).record_llm_call(
                        agent=self._agent,
                        model=self.model,
                        prompt_tokens=getattr(usage, "prompt_tokens", 0),
                        completion_tokens=getattr(usage, "completion_tokens", 0),
                        cost_usd=float(cost),
                        hunt_round=self._hunt_round,
                    )
            except Exception:
                pass  # Never let tracking break the main flow

        return response.choices[0].message.content

    async def close(self) -> None:
        """No-op — litellm manages its own connections."""
        pass
