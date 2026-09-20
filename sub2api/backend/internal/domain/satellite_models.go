package domain

// Default models satellites should send to Sub2API /v1.
// SuperKey still routes across groups; these names are the public catalog.
// Human catalog: docs/卫星公开模型.md
const (
	DefaultTextModel  = "gpt-5.5"
	DefaultImageModel = "gpt-image-2"
	DefaultVideoModel = "grok-imagine-video-1.5"
)

// SatelliteTextModels are the chat/completions and /v1/responses models
// already used by connected apps.
var SatelliteTextModels = []string{
	"gpt-5.5",
	"gpt-5.4-mini",
	"gpt-5.6-luna",
	"deepseek-chat",
}

// SatelliteImageModels come from 模型适配.md / 模型适配实施方案.md.
var SatelliteImageModels = []string{
	"gpt-image-2",
	"gpt-image-1.5",
	"gpt-image-1",
	"gemini-3.1-flash-image",
	"grok-imagine-image-quality",
	"grok-imagine-image-2.0",
	"grok-imagine-image",
	"grok-imagine",
	"grok-imagine-image-1.5",
}

// SatelliteVideoModels come from 计算万物 plus the Grok video adapter.
var SatelliteVideoModels = []string{
	"grok-imagine-video-1.5",
	"seedance-2.0",
	"seedance-2.0-fast",
	"kling-v1",
	"kling-v1-5",
	"kling-v1-6",
	"kling-v2-5-turbo",
	"kling-v2-6",
	"kling-v3",
	"kling-v3-omni",
}
