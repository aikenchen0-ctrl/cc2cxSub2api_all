// Package protocoladapter converts canonical sub-application requests to
// provider-specific compatible protocols.
package protocoladapter

import (
	"encoding/json"
	"fmt"
)

type Profile string

const (
	ProfileOpenAI    Profile = "openai"
	ProfileAnthropic Profile = "anthropic"
	ProfileGemini    Profile = "gemini"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type TextRequest struct {
	Model     string
	System    string
	Messages  []Message
	MaxTokens int
}
type AdaptedRequest struct {
	Path       string
	AuthHeader string
	Body       []byte
}

func AdaptText(req TextRequest, profile Profile) (AdaptedRequest, error) {
	if req.Model == "" {
		return AdaptedRequest{}, fmt.Errorf("model is required")
	}
	switch profile {
	case ProfileOpenAI:
		messages := make([]map[string]string, 0, len(req.Messages)+1)
		if req.System != "" {
			messages = append(messages, map[string]string{"role": "system", "content": req.System})
		}
		for _, message := range req.Messages {
			messages = append(messages, map[string]string{"role": message.Role, "content": message.Content})
		}
		return marshalRequest("/v1/chat/completions", "Authorization", map[string]any{"model": req.Model, "messages": messages})
	case ProfileAnthropic:
		body := map[string]any{"model": req.Model, "messages": req.Messages, "max_tokens": req.MaxTokens}
		if body["max_tokens"].(int) <= 0 {
			body["max_tokens"] = 1024
		}
		if req.System != "" {
			body["system"] = req.System
		}
		return marshalRequest("/v1/messages", "x-api-key", body)
	case ProfileGemini:
		contents := make([]map[string]any, 0, len(req.Messages))
		for _, message := range req.Messages {
			role := "user"
			if message.Role == "assistant" || message.Role == "model" {
				role = "model"
			}
			contents = append(contents, map[string]any{"role": role, "parts": []map[string]string{{"text": message.Content}}})
		}
		body := map[string]any{"contents": contents}
		if req.System != "" {
			body["systemInstruction"] = map[string]any{"parts": []map[string]string{{"text": req.System}}}
		}
		return marshalRequest("/v1beta/models/"+req.Model+":generateContent", "x-goog-api-key", body)
	default:
		return AdaptedRequest{}, fmt.Errorf("unsupported protocol profile: %s", profile)
	}
}

func marshalRequest(path, auth string, body any) (AdaptedRequest, error) {
	data, err := json.Marshal(body)
	return AdaptedRequest{Path: path, AuthHeader: auth, Body: data}, err
}
