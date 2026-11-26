package service

const openAIImageModelUnavailableClientMessage = "图片模型在当前所有上游账号上都不可用（上游返回 404/405，表示该模型在该账号下不存在或未开通）。已自动尝试切换其他账号；请联系管理员为图片分组补充可用上游账号后重试"

// OpenAIImageModelUnavailableClientMessage returns the client message used when
// every eligible image account reported that it cannot serve the requested
// model.
func OpenAIImageModelUnavailableClientMessage() string {
	return openAIImageModelUnavailableClientMessage
}