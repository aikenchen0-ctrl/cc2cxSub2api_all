package modelcatalog

import (
	_ "embed"
	"encoding/json"
)

//go:embed catalog.json
var catalogJSON []byte

var lists = func() map[string][]string {
	var result map[string][]string
	if err := json.Unmarshal(catalogJSON, &result); err != nil {
		panic(err)
	}
	return result
}()

// ForPlatform uses the same catalog and aliases as the editor's fillRelated action.
func ForPlatform(platform string) []string {
	key := map[string]string{
		"openai": "openaiModels", "anthropic": "claudeModels", "claude": "claudeModels",
		"gemini": "geminiModels", "antigravity": "antigravityModels", "zhipu": "zhipuModels",
		"qwen": "qwenModels", "deepseek": "deepseekModels", "mistral": "mistralModels",
		"meta": "metaModels", "xai": "xaiModels", "grok": "xaiModels", "cohere": "cohereModels",
		"yi": "yiModels", "moonshot": "moonshotModels", "kimi": "moonshotModels",
		"opencode_go": "opencodeGoModels", "doubao": "doubaoModels", "minimax": "minimaxModels",
		"baidu": "baiduModels", "spark": "sparkModels", "hunyuan": "hunyuanModels",
		"perplexity": "perplexityModels",
	}[platform]
	if key == "" {
		key = "claudeModels"
	}
	return append([]string(nil), lists[key]...)
}
