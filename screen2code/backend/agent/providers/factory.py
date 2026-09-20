import os
from contextvars import ContextVar
from typing import Optional

from anthropic import AsyncAnthropic
from google import genai
from openai import AsyncOpenAI
from openai.types.chat import ChatCompletionMessageParam

from agent.providers.anthropic import AnthropicProviderSession, serialize_anthropic_tools
from agent.providers.base import ProviderSession
from agent.providers.gemini import GeminiProviderSession, serialize_gemini_tools
from agent.providers.openai import OpenAIProviderSession, serialize_openai_tools
from agent.tools import canonical_tool_definitions
from config import REPLICATE_API_KEY
from fs_logging.agent_runs import AgentRunRecorder
from llm import ANTHROPIC_MODELS, GEMINI_MODELS, OPENAI_MODELS, Llm
from preview_screenshot import is_screenshot_preview_available

satellite_on_behalf_of: ContextVar[str] = ContextVar("satellite_on_behalf_of", default="")


def create_provider_session(
    model: Llm,
    prompt_messages: list[ChatCompletionMessageParam],
    should_generate_images: bool,
    openai_api_key: Optional[str],
    openai_base_url: Optional[str],
    anthropic_api_key: Optional[str],
    gemini_api_key: Optional[str],
    replicate_api_key: Optional[str],
    should_extract_assets: bool = True,
    recorder: Optional[AgentRunRecorder] = None,
) -> ProviderSession:
    canonical_tools = canonical_tool_definitions(
        image_generation_enabled=should_generate_images,
        # The edit_images tool calls Replicate, so don't offer it without a key.
        image_editing_enabled=bool(replicate_api_key or REPLICATE_API_KEY),
        # The extract_assets tool calls Gemini, so don't offer it without a key.
        asset_extraction_enabled=should_extract_assets and bool(gemini_api_key),
        # screenshot_preview needs headless Chromium; skip it if it can't launch.
        screenshot_enabled=is_screenshot_preview_available(),
    )

    if model in OPENAI_MODELS:
        headers = {}
        credential = os.environ.get("SUB2API_APP_CREDENTIAL", "").strip()
        subject = satellite_on_behalf_of.get().strip()
        managed = bool(credential or os.environ.get("SUB2API_SSO_SECRET", "").strip())
        api_key = openai_api_key
        if managed and (not credential or not subject):
            raise Exception("Sub2API satellite credential and session subject are required.")
        if credential:
            headers["X-Sub2API-On-Behalf-Of"] = subject
            headers["X-Sub2API-Satellite"] = "screen2code"
            api_key = credential
        if not api_key:
            raise Exception("OpenAI API key is missing.")
        client = AsyncOpenAI(
            api_key=api_key,
            base_url=openai_base_url,
            default_headers=headers or None,
        )
        return OpenAIProviderSession(
            client=client,
            model=model,
            prompt_messages=prompt_messages,
            tools=serialize_openai_tools(canonical_tools),
            recorder=recorder,
        )

    if model in ANTHROPIC_MODELS:
        if not anthropic_api_key:
            raise Exception("Anthropic API key is missing.")

        client = AsyncAnthropic(api_key=anthropic_api_key)
        return AnthropicProviderSession(
            client=client,
            model=model,
            prompt_messages=prompt_messages,
            tools=serialize_anthropic_tools(canonical_tools),
            recorder=recorder,
        )

    if model in GEMINI_MODELS:
        if not gemini_api_key:
            raise Exception("Gemini API key is missing.")

        client = genai.Client(api_key=gemini_api_key)
        return GeminiProviderSession(
            client=client,
            model=model,
            prompt_messages=prompt_messages,
            tools=serialize_gemini_tools(canonical_tools),
            recorder=recorder,
        )

    raise ValueError(f"Unsupported model: {model.value}")
