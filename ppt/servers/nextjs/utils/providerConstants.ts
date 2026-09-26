export interface ModelOption {
  value: string;
  label: string;
  description?: string;
  icon?: string;
  size: string;
}

export interface ImageProviderOption {
  value: string;
  label: string;
  description?: string;
  icon?: string;
  requiresApiKey?: boolean;
  apiKeyField?: string;
  apiKeyFieldLabel?: string;
  getApiKeyUrl?: string;
}

export interface LLMProviderOption {
  value: string;
  label: string;
  description?: string;
  model_value?: string;
  model_label?: string;
  url?: string;
  icon?: string;
  getApiKeyUrl?: string;
}

export interface WebSearchProviderOption {
  value: string;
  label: string;
  description: string;
  icon?: string;
  apiKeyField?: string;
  apiKeyLabel?: string;
  urlField?: string;
  urlLabel?: string;
}

export const WEB_SEARCH_PROVIDERS: Record<string, WebSearchProviderOption> = {
  auto: {
    value: "auto",
    label: "默认（模型）",
    description:
      "优先使用模型自带的联网能力；否则需选择外部服务商后才会启用网页搜索。",
    icon: "/providers/model-search.svg",
  },
  searxng: {
    value: "searxng",
    label: "SearXNG",
    description: "使用自托管的 SearXNG 实例。",
    icon: "/providers/searxng.svg",
    urlField: "SEARXNG_BASE_URL",
    urlLabel: "SearXNG 基础地址",
  },
  tavily: {
    value: "tavily",
    label: "Tavily",
    description: "面向 AI 应用优化的搜索 API。",
    icon: "/providers/tavily.png",
    apiKeyField: "TAVILY_API_KEY",
    apiKeyLabel: "Tavily API 密钥",
  },
  exa: {
    value: "exa",
    label: "Exa",
    description: "原生面向 AI 的网页搜索，并提取结果摘要。",
    icon: "/providers/exa.png",
    apiKeyField: "EXA_API_KEY",
    apiKeyLabel: "Exa API 密钥",
  },
  brave: {
    value: "brave",
    label: "Brave",
    description: "用于网页搜索结果的 Brave Search API。",
    icon: "/providers/brave.svg",
    apiKeyField: "BRAVE_SEARCH_API_KEY",
    apiKeyLabel: "Brave Search API 密钥",
  },
  // serper: {
  //   value: "serper",
  //   label: "Serper",
  //   description: "Google search results via Serper.",
  //   apiKeyField: "SERPER_API_KEY",
  //   apiKeyLabel: "Serper API key",
  // },
};

export const IMAGE_PROVIDERS: Record<string, ImageProviderOption> = {
  pexels: {
    value: "pexels",
    label: "Pexels",
    description: "免费图库和视频平台",
    icon: "/providers/pexel.png",
    requiresApiKey: true,
    apiKeyField: "PEXELS_API_KEY",
    apiKeyFieldLabel: "Pexels API 密钥",
    getApiKeyUrl: "https://docs.presenton.ai/help/get-api-keys/get-pexels-api-key",
  },
  pixabay: {
    value: "pixabay",
    label: "Pixabay",
    description: "免费图片和视频",
    icon: "/providers/pixabay.png",
    requiresApiKey: true,
    apiKeyField: "PIXABAY_API_KEY",
    apiKeyFieldLabel: "Pixabay API 密钥",
    getApiKeyUrl: "https://docs.presenton.ai/help/get-api-keys/get-pixabay-api-key",
  },
  "dall-e-3": {
    value: "dall-e-3",
    label: "DALL-E 3",
    description: "OpenAI 图片生成模型",
    icon: "/providers/openai.png",
    requiresApiKey: true,
    apiKeyField: "OPENAI_API_KEY",
    apiKeyFieldLabel: "OpenAI API 密钥",
    getApiKeyUrl: "https://www.google.com/search?q=how+to+get+openai+api+key&ie=UTF-8",
  },
  "gpt-image-1.5": {
    value: "gpt-image-1.5",
    label: "GPT Image 1.5",
    description: "OpenAI's image generation model",
    icon: "/providers/openai.png",
    requiresApiKey: true,
    apiKeyField: "OPENAI_API_KEY",
    apiKeyFieldLabel: "OpenAI API Key",
    getApiKeyUrl: "https://www.google.com/search?q=how+to+get+openai+api+key&ie=UTF-8",
  },
  gemini_flash: {
    value: "gemini_flash",
    label: "Gemini Flash",
    description: "Google 快速图片生成模型",
    icon: "/providers/gemini-color.svg",
    requiresApiKey: true,
    apiKeyField: "GOOGLE_API_KEY",
    apiKeyFieldLabel: "Google API 密钥",
    getApiKeyUrl: "https://www.google.com/search?q=how+to+get+google+AI+studio+api+key&sxsrf=ANbL-n5_hUGaEiG9v6k9VxZWyv0mqO0Jew%3A1776339625724",
  },
  nanobanana_pro: {
    value: "nanobanana_pro",
    label: "NanoBanana Pro",
    description: "Google 高级图片生成模型",
    icon: "/providers/gemini-color.svg",
    requiresApiKey: true,
    apiKeyField: "GOOGLE_API_KEY",
    apiKeyFieldLabel: "Google API Key",
    getApiKeyUrl: "https://www.google.com/search?q=how+to+get+google+AI+studio+api+key&sxsrf=ANbL-n5_hUGaEiG9v6k9VxZWyv0mqO0Jew%3A1776339625724",
  },
  comfyui: {
    value: "comfyui",
    label: "ComfyUI",
    description: "使用本地 ComfyUI 服务器和自定义工作流",
    icon: "/providers/comfyui-color.svg",
    requiresApiKey: false,
    apiKeyField: "COMFYUI_URL",
    apiKeyFieldLabel: "ComfyUI 服务器地址",
  },
  open_webui: {
    value: "open_webui",
    label: "Open WebUI",
    description: "使用 Open WebUI 服务器生成图片",
    icon: "/providers/open-webui.png",
    requiresApiKey: false,
    apiKeyField: "OPEN_WEBUI_IMAGE_URL",
    apiKeyFieldLabel: "Open WebUI 地址",
  },
  openai_compatible: {
    value: "openai_compatible",
    label: "自定义",
    description:
      "兼容 OpenAI 的 /v1/images 接口（LiteLLM、Azure、vLLM 等）",
    icon: "/providers/custom.svg",
    requiresApiKey: false,
    apiKeyField: "OPENAI_COMPAT_IMAGE_BASE_URL",
    apiKeyFieldLabel: "OpenAI 兼容基础地址",
  },
};

export const LLM_PROVIDERS: Record<string, LLMProviderOption> = {
  presenton: {
    value: "presenton",
    label: "永恒PPT",
    description: "使用已连接的永恒PPT云端账号生成",
    icon: "/providers/presenton.png",
  },
  codex: {
    value: "codex",
    label: "ChatGPT",
    description: "ChatGPT Plus/Pro via OAuth",
    icon: "/providers/openai.png",
  },
  openai: {
    value: "openai",
    label: "OpenAI",
    description: "OpenAI's latest text generation model",
    url: "https://api.openai.com/v1",
    icon: "/providers/openai.png",
    getApiKeyUrl: "https://www.google.com/search?q=how+to+get+openai+api+key&ie=UTF-8",
  },
  deepseek: {
    value: "deepseek",
    label: "DeepSeek",
    description: "DeepSeek models via DeepSeek API",
    url: "https://api.deepseek.com/v1",
    icon: "/providers/deepseek-color.svg",
    getApiKeyUrl: "https://platform.deepseek.com/api_keys",
  },
  google: {
    value: "google",
    label: "Google",
    description: "Google's primary text generation model",
    url: "https://api.google.com/v1",
    icon: "/providers/gemini-color.svg",
    getApiKeyUrl: "https://www.google.com/search?q=how+to+get+google+AI+studio+api+key&sxsrf=ANbL-n5_hUGaEiG9v6k9VxZWyv0mqO0Jew%3A1776339625724",
  },
  vertex: {
    value: "vertex",
    label: "Vertex AI",
    description: "Google Vertex AI models",
    icon: "/providers/vertexai-color.svg",
    getApiKeyUrl: "https://www.google.com/search?q=how+to+get+vertex+ai+api+key",
  },
  azure: {
    value: "azure",
    label: "Azure OpenAI",
    description: "Azure-hosted OpenAI deployments",
    icon: "/providers/azure-color.svg",
    getApiKeyUrl: "https://www.google.com/search?q=azure+openai+api+key",
  },
  bedrock: {
    value: "bedrock",
    label: "Amazon Bedrock",
    description: "AWS Bedrock foundation models",
    icon: "/providers/bedrock-color.svg",
  },
  openrouter: {
    value: "openrouter",
    label: "OpenRouter",
    description: "Many models through OpenRouter’s OpenAI-compatible API",
    url: "https://openrouter.ai/api/v1",
    icon: "/providers/openrouter-color.svg",
    getApiKeyUrl: "https://openrouter.ai/keys",
  },
  cerebras: {
    value: "cerebras",
    label: "Cerebras",
    description: "Cerebras Cloud via OpenAI-compatible API",
    url: "https://api.cerebras.ai/v1",
    icon: "/providers/cerebras-color.svg",
    getApiKeyUrl: "https://inference-docs.cerebras.ai",
  },
  litellm: {
    value: "litellm",
    label: "LiteLLM",
    description: "OpenAI-compatible LiteLLM proxy or gateway",
    icon: "/providers/litellm-logo.svg",
  },
  fireworks: {
    value: "fireworks",
    label: "Fireworks",
    description: "Fireworks AI via OpenAI-compatible API",
    url: "https://api.fireworks.ai/inference/v1",
    icon: "/providers/fireworks-color.svg",
    getApiKeyUrl: "https://fireworks.ai/account/api-keys",
  },
  together: {
    value: "together",
    label: "Together AI",
    description: "Together AI via OpenAI-compatible API",
    url: "https://api.together.ai/v1",
    icon: "/providers/together-color.svg",
    getApiKeyUrl: "https://api.together.xyz/settings/api-keys",
  },
  lmstudio: {
    value: "lmstudio",
    label: "LM Studio",
    description: "Local LM Studio OpenAI-compatible server",
    url: "http://localhost:1234/v1",
    icon: "/providers/lm-studio.svg",
  },
  anthropic: {
    value: "anthropic",
    label: "Anthropic",
    description: "Anthropic's Claude models",
    url: "https://api.anthropic.com/v1",
    icon: "/providers/claude-color.svg",
    getApiKeyUrl: "https://www.google.com/search?q=how+to+get+anthropic+api+key&sxsrf=ANbL-n7lsueZQ88L56HhqC1ch2PGD0rbNQ%3A1776339632265",
  },
  ollama: {
    value: "ollama",
    label: "Ollama",
    description: "Ollama's primary text generation model",
    icon: "/providers/ollama.svg",
  },
  custom: {
    value: "custom",
    label: "Custom",
    description: "OpenAI-compatible LLM",
    icon: "/providers/custom.svg",
  },

};

export const DALLE_3_QUALITY_OPTIONS = [
  {
    label: "Standard",
    value: "standard",
    description: "Faster generation with lower cost",
  },
  {
    label: "HD",
    value: "hd",
    description: "Higher quality images with increased cost",
  },
];

export const GPT_IMAGE_1_5_QUALITY_OPTIONS = [
  {
    label: "Low",
    value: "low",
    description: "Fastest and most cost-effective",
  },
  {
    label: "Medium",
    value: "medium",
    description: "Balanced quality and speed",
  },
  {
    label: "High",
    value: "high",
    description: "Best quality with longer generation time",
  },
];
