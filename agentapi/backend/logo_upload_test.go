package main

import (
	"bytes"
	"encoding/base64"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBrandLogoUploadStoresBase64ImageAndRejectsMarkup(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("logo upload reached upstream: %s", r.URL.Path)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	sessionID, err := server.store.CreateSession("42", []byte(`{"id":42}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0}
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "../evil.svg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(png); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "http://agent.local/api/v1/agent/admin/branding/logo", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Origin", "http://agent.local")
	req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "data:image/png;base64,") {
		t.Fatalf("upload status=%d body=%s", rec.Code, rec.Body.String())
	}
	agent, err := server.store.Agent(server.cfg.AgentID)
	if err != nil {
		t.Fatal(err)
	}
	wantLogo := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
	if agent.SiteLogo != wantLogo || !strings.HasPrefix(agent.SiteLogo, "data:image/png;base64,") {
		t.Fatalf("logo was not stored as a base64 data URL: %q", agent.SiteLogo)
	}
	svg := httptest.NewRequest(http.MethodPost, "http://agent.local/api/v1/agent/admin/branding/logo", strings.NewReader("not-an-image"))
	svg.Header.Set("Origin", "http://agent.local")
	svg.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	svgRec := httptest.NewRecorder()
	server.ServeHTTP(svgRec, svg)
	if svgRec.Code != http.StatusBadRequest {
		t.Fatalf("invalid upload status=%d body=%s", svgRec.Code, svgRec.Body.String())
	}
}

func TestBrandLogoValidationAcceptsOnlyMatchingEmbeddedImages(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0}
	valid := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
	if err := validateBrandLogo(valid); err != nil {
		t.Fatalf("valid embedded logo was rejected: %v", err)
	}
	for _, value := range []string{
		"data:text/html;base64,PGgxPm5vdCBhbiBpbWFnZTwvaDE+",
		"data:image/svg+xml;base64,PHN2Zz48L3N2Zz4=",
		"data:image/png;base64,not-base64",
		"data:image/gif;base64," + base64.StdEncoding.EncodeToString(png),
		"javascript:alert(1)",
	} {
		if err := validateBrandLogo(value); err == nil {
			t.Fatalf("unsafe embedded logo was accepted: %s", value)
		}
	}
}
