package service

import (
	"strings"
	"testing"
)

func TestHTMLPageTitle(t *testing.T) {
	tests := []struct {
		name string
		page string
		want string
	}{
		{
			name: "simple title",
			page: "<!doctype html><html><head><title>\u767b\u5f55\u00b7MoMo \u9762\u677f</title></head></html>",
			want: "\u767b\u5f55\u00b7MoMo \u9762\u677f",
		},
		{
			name: "uppercase tag",
			page: "<HTML><TITLE>Sign in</TITLE></HTML>",
			want: "Sign in",
		},
		{
			name: "tag with attributes and whitespace",
			page: "<title id=\"x\">\n  Login   Page \n</title>",
			want: "Login Page",
		},
		{
			name: "long title truncated",
			page: "<title>" + strings.Repeat("a", 80) + "</title>",
			want: strings.Repeat("a", 60) + "...",
		},
		{
			name: "no title",
			page: "<html><body>blocked</body></html>",
			want: "",
		},
		{
			name: "empty title",
			page: "<title></title>",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HTMLPageTitle([]byte(tt.page)); got != tt.want {
				t.Fatalf("HTMLPageTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOpenAIImagesUpstreamHTMLPageErrorBody(t *testing.T) {
	body := OpenAIImagesUpstreamHTMLPageErrorBody([]byte("<html><title>Login</title></html>"))
	if !IsOpenAIUpstreamHTMLPageErrorBody(body) {
		t.Fatalf("expected body to be detected as html page error: %s", body)
	}
	if !strings.Contains(string(body), "Login") {
		t.Fatalf("expected page title in message: %s", body)
	}

	withoutTitle := OpenAIImagesUpstreamHTMLPageErrorBody([]byte("<html></html>"))
	if !IsOpenAIUpstreamHTMLPageErrorBody(withoutTitle) {
		t.Fatalf("expected body to be detected without title: %s", withoutTitle)
	}
	if strings.Contains(string(withoutTitle), "()") {
		t.Fatalf("unexpected empty title suffix: %s", withoutTitle)
	}
}

func TestIsOpenAIUpstreamHTMLPageErrorBodyNegative(t *testing.T) {
	cases := [][]byte{
		[]byte(`{"error":{"type":"upstream_error","code":"insufficient_quota","message":"no quota"}}`),
		[]byte(`{"data":[]}`),
		[]byte(`not json at all`),
		[]byte(``),
	}
	for _, body := range cases {
		if IsOpenAIUpstreamHTMLPageErrorBody(body) {
			t.Fatalf("unexpected match for body: %s", body)
		}
	}

	// Truncated payloads still match through the raw substring fallback.
	if !IsOpenAIUpstreamHTMLPageErrorBody([]byte(`{"error":{"code":"upstream_html_page"`)) {
		t.Fatal("expected fallback substring match")
	}
}

func TestOpenAIUpstreamHTMLPageClientMessage(t *testing.T) {
	msg := OpenAIUpstreamHTMLPageClientMessage()
	if !strings.Contains(msg, "\u4e0a\u6e38\u6e20\u9053") || !strings.Contains(msg, "\u767b\u5f55") {
		t.Fatalf("unexpected client message: %s", msg)
	}
	if OpenAIUpstreamHTMLPageCode != "upstream_html_page" {
		t.Fatalf("unexpected code: %s", OpenAIUpstreamHTMLPageCode)
	}
}