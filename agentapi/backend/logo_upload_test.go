package main

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBrandLogoUploadStoresImageAndRejectsMarkup(t *testing.T) {
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
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `/branding/logos/`+server.cfg.AgentID+`.png`) {
		t.Fatalf("upload status=%d body=%s", rec.Code, rec.Body.String())
	}
	stored := filepath.Join(filepath.Dir(server.cfg.DatabasePath), "logos", server.cfg.AgentID+".png")
	if _, err := os.Stat(stored); err != nil {
		t.Fatal(err)
	}
	get := httptest.NewRequest(http.MethodGet, "http://agent.local/branding/logos/"+server.cfg.AgentID+".png", nil)
	got := httptest.NewRecorder()
	server.ServeHTTP(got, get)
	if got.Code != http.StatusOK || got.Header().Get("Content-Type") != "image/png" || !bytes.Equal(got.Body.Bytes(), png) {
		t.Fatalf("serve status=%d type=%s", got.Code, got.Header().Get("Content-Type"))
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
