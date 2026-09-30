package service

import "testing"

func TestCreateClaudeTestPayloadUsesAuditPrompt(t *testing.T) {
	payload, err := createTestPayload("claude-opus-5", "choose 300 integers")
	if err != nil {
		t.Fatal(err)
	}
	messages := payload["messages"].([]map[string]any)
	content := messages[0]["content"].([]map[string]any)
	if content[0]["text"] != "choose 300 integers" || payload["max_tokens"] != 2048 {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestCreateOpenAITestPayloadUsesAuditPrompt(t *testing.T) {
	payload := createOpenAITestPayload("gpt-5.4", false, "choose 300 integers")
	input := payload["input"].([]map[string]any)
	content := input[0]["content"].([]map[string]any)
	if content[0]["text"] != "choose 300 integers" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestDefaultTestPayloadsRemainSmall(t *testing.T) {
	claudePayload, err := createTestPayload("claude-opus-5")
	if err != nil {
		t.Fatal(err)
	}
	if claudePayload["max_tokens"] != 1024 {
		t.Fatalf("default max_tokens = %#v", claudePayload["max_tokens"])
	}
	openAIPayload := createOpenAITestPayload("gpt-5.4", false)
	input := openAIPayload["input"].([]map[string]any)
	content := input[0]["content"].([]map[string]any)
	if content[0]["text"] != "hi" {
		t.Fatalf("default prompt = %#v", content[0]["text"])
	}
}
