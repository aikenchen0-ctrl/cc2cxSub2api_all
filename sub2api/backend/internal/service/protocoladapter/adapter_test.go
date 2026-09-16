package protocoladapter

import (
	"encoding/json"
	"testing"
)

func TestAdaptTextRequestForSupportedProtocols(t *testing.T) {
	input := TextRequest{Model: "demo", System: "be concise", Messages: []Message{{Role: "user", Content: "hello"}}}
	tests := []struct {
		name    string
		profile Profile
		path    string
		auth    string
		check   func(map[string]any) bool
	}{
		{"openai", ProfileOpenAI, "/v1/chat/completions", "Authorization", func(body map[string]any) bool {
			messages := body["messages"].([]any)
			return body["model"] == "demo" && messages[1].(map[string]any)["content"] == "hello"
		}},
		{"anthropic", ProfileAnthropic, "/v1/messages", "x-api-key", func(body map[string]any) bool { return body["model"] == "demo" && body["system"] == "be concise" }},
		{"gemini", ProfileGemini, "/v1beta/models/demo:generateContent", "x-goog-api-key", func(body map[string]any) bool { return body["contents"].([]any)[0].(map[string]any)["role"] == "user" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request, err := AdaptText(input, tt.profile)
			if err != nil || request.Path != tt.path || request.AuthHeader != tt.auth {
				t.Fatalf("request = %#v, err = %v", request, err)
			}
			var body map[string]any
			if err := json.Unmarshal(request.Body, &body); err != nil {
				t.Fatal(err)
			}
			if !tt.check(body) {
				t.Fatalf("body = %#v", body)
			}
		})
	}
}

func TestAdaptTextRejectsUnknownProtocol(t *testing.T) {
	_, err := AdaptText(TextRequest{Model: "demo"}, Profile("unknown"))
	if err == nil {
		t.Fatal("expected unsupported profile error")
	}
}
