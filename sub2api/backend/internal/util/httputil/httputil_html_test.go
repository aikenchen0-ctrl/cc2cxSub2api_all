package httputil

import (
	"net/http"
	"testing"
)

func TestLooksLikeHTMLResponse(t *testing.T) {
	tests := []struct {
		name    string
		headers http.Header
		body    string
		want    bool
	}{
		{
			name:    "html content type",
			headers: http.Header{"Content-Type": []string{"text/html"}},
			body:    "anything",
			want:    true,
		},
		{
			name:    "html content type with charset",
			headers: http.Header{"Content-Type": []string{"text/html; charset=utf-8"}},
			body:    "anything",
			want:    true,
		},
		{
			name:    "xhtml content type",
			headers: http.Header{"Content-Type": []string{"application/xhtml+xml"}},
			body:    "anything",
			want:    true,
		},
		{
			name:    "doctype body without headers",
			headers: nil,
			body:    "<!doctype html><html><title>login</title></html>",
			want:    true,
		},
		{
			name:    "leading whitespace and bom",
			headers: http.Header{},
			body:    "\ufeff\n  <html><body>x</body></html>",
			want:    true,
		},
		{
			name:    "json body",
			headers: http.Header{"Content-Type": []string{"application/json"}},
			body:    `{"data":[{"b64_json":"abc"}]}`,
			want:    false,
		},
		{
			name:    "json body with plain text content type",
			headers: http.Header{"Content-Type": []string{"text/plain"}},
			body:    `{"error":{"message":"slow down"}}`,
			want:    false,
		},
		{
			name:    "binary image payload",
			headers: http.Header{"Content-Type": []string{"image/png"}},
			body:    "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR",
			want:    false,
		},
		{
			name:    "empty body",
			headers: http.Header{"Content-Type": []string{"application/json"}},
			body:    "",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LooksLikeHTMLResponse(tt.headers, []byte(tt.body)); got != tt.want {
				t.Fatalf("LooksLikeHTMLResponse() = %v, want %v", got, tt.want)
			}
		})
	}
}