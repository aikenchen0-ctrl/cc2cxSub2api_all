package service

import (
	"encoding/json"
	"strings"

	"github.com/tidwall/gjson"
)

// OpenAIContinuationSafety 只判断当前正文是否可独立重放，不根据客户端
// 名称、消息轮数或角色猜测远端历史是否齐全。它不会修改请求正文。
type OpenAIContinuationSafety struct {
	PreviousResponseID         string
	CanReplayAcrossAccounts    bool
	CanStripEncryptedReasoning bool
	HasOpaqueCompaction        bool
	HasRemoteReferences        bool
	HasEncryptedReasoning      bool
	HasPlaintextInput          bool
}

// 同账号收到推理密文校验失败时，可保留原响应标识清理普通推理密文。
// 压缩历史不可重建；没有明文输入或原响应锚点的纯密文也不能直接删除。
func (s OpenAIContinuationSafety) CanRecoverRejectedEncryptedReasoning() bool {
	return s.HasEncryptedReasoning && !s.HasOpaqueCompaction && !s.HasRemoteReferences &&
		(s.CanReplayAcrossAccounts || s.PreviousResponseID != "")
}

// 只有亲自观察的完整父轮且响应标识匹配，才能用本地回放替代远端锚点。
// 首轮外来标识与当前会话不相连，不能把其增量误当作完整历史。
func openAIWSReplayHistoryComplete(payload []byte, replay []json.RawMessage, replayExists bool, previousComplete bool, previousResponseID string) bool {
	safety := AnalyzeOpenAIContinuationSafety(payload)
	if safety.PreviousResponseID == "" {
		if safety.CanReplayAcrossAccounts {
			return true
		}
		// 同一连接上的工具结果可能不带响应标识。仅在父轮完整且
		// 本轮缺少的调用正文能从已观察历史补齐时，允许按 call_id
		// 恢复上下文；不能把一般新请求或远端引用当作父轮增量。
		coverage := AnalyzeToolCallOutputContextCoverageBytes(payload)
		if !previousComplete || !coverage.HasFunctionCallOutput || coverage.ContextCoversAllCallIDs {
			return false
		}
		fullBody, err := setOpenAIWSPayloadInputSequence(payload, replay, replayExists)
		return err == nil && AnalyzeOpenAIContinuationSafety(fullBody).CanReplayAcrossAccounts
	}
	if !previousComplete || previousResponseID == "" || safety.PreviousResponseID != previousResponseID {
		return false
	}
	fullBody, err := setOpenAIWSPayloadInputSequence(payload, replay, replayExists)
	if err != nil {
		return false
	}
	fullBody, _, err = dropPreviousResponseIDFromRawPayload(fullBody)
	if err != nil {
		return false
	}
	return AnalyzeOpenAIContinuationSafety(fullBody).CanReplayAcrossAccounts
}

// AnalyzeOpenAIContinuationSafety 将远端状态和普通推理密文分开：已有明文
// 历史的无状态请求可沿用推理兼容清理；压缩密文、远端引用和旧响应标识
// 不能靠删字段恢复。即使带有若干旧消息，也无法证明旧响应历史已全部回放。
func AnalyzeOpenAIContinuationSafety(body []byte) OpenAIContinuationSafety {
	result := OpenAIContinuationSafety{}
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return result
	}
	root := gjson.ParseBytes(body)
	if !root.IsObject() {
		return result
	}
	// 冲突关键字段会让 JSON 解析器对历史来源产生不同结论。
	// 完全相同的响应锚点可由严格删除器清理；其余重复键不猜测
	// 首值或末值，保留原文并禁用本地重建与密文清理。
	criticalFields := make(map[string]string, 3)
	ambiguous := false
	root.ForEach(func(key, value gjson.Result) bool {
		switch key.String() {
		case "previous_response_id", "conversation", "input":
			if prior, exists := criticalFields[key.String()]; exists {
				if key.String() != "previous_response_id" || prior != value.Raw {
					ambiguous = true
				}
			}
			criticalFields[key.String()] = value.Raw
		}
		if key.String() == "previous_response_id" && value.Type != gjson.String && value.Type != gjson.Null {
			ambiguous = true
		}
		return true
	})
	if ambiguous {
		result.HasRemoteReferences = true
		return result
	}
	result.PreviousResponseID = strings.TrimSpace(root.Get("previous_response_id").String())
	conversation := root.Get("conversation")
	if conversation.Exists() && conversation.Type != gjson.Null && strings.TrimSpace(conversation.String()) != "" {
		result.HasRemoteReferences = true
	}
	input := root.Get("input")
	if input.Type == gjson.String && strings.TrimSpace(input.String()) != "" {
		result.HasPlaintextInput = true
	}
	var inspect func(gjson.Result, int)
	inspect = func(value gjson.Result, depth int) {
		if !value.IsArray() && !value.IsObject() {
			return
		}
		// 极端嵌套视为无法验证，既避免栈消耗，也不放宽状态安全边界。
		if depth >= 32 {
			result.HasRemoteReferences = true
			return
		}
		if value.IsObject() {
			typ := strings.TrimSpace(value.Get("type").String())
			switch typ {
			case "item_reference":
				result.HasRemoteReferences = true
			case "compaction", "compaction_summary":
				result.HasOpaqueCompaction = true
			case "reasoning":
				if value.Get("encrypted_content").Exists() {
					result.HasEncryptedReasoning = true
				}
			case "input_text", "output_text":
				if strings.TrimSpace(value.Get("text").String()) != "" {
					result.HasPlaintextInput = true
				}
			default:
				if value.Get("encrypted_content").Exists() {
					result.HasRemoteReferences = true
				}
			}
			if typ == "message" || (typ == "" && value.Get("role").Exists()) {
				content := value.Get("content")
				if (content.Type == gjson.String && strings.TrimSpace(content.String()) != "") ||
					(content.IsArray() && len(content.Array()) > 0) {
					result.HasPlaintextInput = true
				}
			}
			if isCodexToolCallContextItemType(typ) || isCodexToolCallOutputItemType(typ) {
				result.HasPlaintextInput = true
			}
			for _, field := range []string{"file_id", "image_file_id"} {
				if strings.TrimSpace(value.Get(field).String()) != "" {
					result.HasRemoteReferences = true
				}
			}
		}
		value.ForEach(func(_, item gjson.Result) bool {
			inspect(item, depth+1)
			return true
		})
	}
	inspect(input, 0)
	coverage := AnalyzeToolCallOutputContextCoverageBytes(body)
	toolContextComplete := !coverage.HasFunctionCallOutput || coverage.ContextCoversAllCallIDs
	result.CanReplayAcrossAccounts = result.PreviousResponseID == "" &&
		!result.HasRemoteReferences && !result.HasOpaqueCompaction && toolContextComplete &&
		(!result.HasEncryptedReasoning || result.HasPlaintextInput)
	result.CanStripEncryptedReasoning = result.CanReplayAcrossAccounts && result.HasEncryptedReasoning
	return result
}
