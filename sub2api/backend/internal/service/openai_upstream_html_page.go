package service

import (
	"bytes"
	"encoding/json"
	"strings"
)

// OpenAIUpstreamHTMLPageCode marks a synthesized upstream error that stands for
// "the upstream answered with a web page instead of an API payload".
const OpenAIUpstreamHTMLPageCode = "upstream_html_page"

const openAIUpstreamHTMLPageClientMessage = "上游渠道返回的是网页（登录页或错误页）而不是接口数据，通常是该上游账号登录状态已失效，请联系管理员处理该上游账号后重试"

// OpenAIImagesUpstreamHTMLPageErrorBody builds the failover body used when an
// images upstream answers a 2xx request with an HTML page. Only the page title
// is carried over so logs and client responses never echo the page itself.
func OpenAIImagesUpstreamHTMLPageErrorBody(page []byte) []byte {
	message := "upstream returned an HTML page instead of an image API response"
	if title := HTMLPageTitle(page); title != "" {
		message += " (" + title + ")"
	}
	encoded, err := json.Marshal(map[string]any{
		"error": map[string]any{
			"type":    "upstream_error",
			"code":    OpenAIUpstreamHTMLPageCode,
			"message": message,
		},
	})
	if err != nil {
		return []byte(`{"error":{"type":"upstream_error","code":"upstream_html_page","message":"upstream returned an HTML page instead of an image API response"}}`)
	}
	return encoded
}

// IsOpenAIUpstreamHTMLPageErrorBody reports whether a failover body was
// produced by the HTML-page detector.
func IsOpenAIUpstreamHTMLPageErrorBody(body []byte) bool {
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &payload) != nil {
		return bytes.Contains(body, []byte(`"code":"`+OpenAIUpstreamHTMLPageCode+`"`))
	}
	return strings.TrimSpace(payload.Error.Code) == OpenAIUpstreamHTMLPageCode
}

// OpenAIUpstreamHTMLPageClientMessage returns the exhausted-failover client
// message for upstreams that answer with a web page.
func OpenAIUpstreamHTMLPageClientMessage() string {
	return openAIUpstreamHTMLPageClientMessage
}

// HTMLPageTitle extracts a short <title> from an HTML page for diagnostics.
func HTMLPageTitle(page []byte) string {
	lower := bytes.ToLower(page)
	start := bytes.Index(lower, []byte("<title"))
	if start < 0 {
		return ""
	}
	open := bytes.IndexByte(lower[start:], '>')
	if open < 0 {
		return ""
	}
	content := page[start+open+1:]
	if end := bytes.Index(bytes.ToLower(content), []byte("</title>")); end >= 0 {
		content = content[:end]
	}
	title := strings.Join(strings.Fields(string(content)), " ")
	if utf8RuneCount(title) > 60 {
		title = string([]rune(title)[:60]) + "..."
	}
	return title
}

func utf8RuneCount(value string) int {
	count := 0
	for range value {
		count++
	}
	return count
}
