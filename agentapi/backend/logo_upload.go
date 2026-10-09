package main

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const maxLogoUploadBytes = 1 << 20

var logoAgentIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`)

func (s *Server) handleBrandLogoUpload(w http.ResponseWriter, r *http.Request, requestID string, tenant TenantContext) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	if !sameOrigin(r) {
		s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin state-changing requests are not allowed")
		return
	}
	if !logoAgentIDPattern.MatchString(tenant.AgentID) {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_AGENT", "current station cannot store an uploaded logo")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxLogoUploadBytes+8192)
	if err := r.ParseMultipartForm(maxLogoUploadBytes); err != nil {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_LOGO", "logo upload must be an image of at most 1 MB")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_LOGO", "choose an image file to upload")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxLogoUploadBytes+1))
	if err != nil || len(data) == 0 || len(data) > maxLogoUploadBytes {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_LOGO", "logo upload must be an image of at most 1 MB")
		return
	}
	ext, contentType, ok := detectLogoType(data)
	if !ok {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_LOGO", "logo must be a PNG, JPEG, WEBP, or GIF image")
		return
	}
	dir := filepath.Join(filepath.Dir(s.cfg.DatabasePath), "logos")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "LOGO_STORE_FAILED", "failed to store logo")
		return
	}
	name := tenant.AgentID + ext
	target := filepath.Join(dir, name)
	temp := target + ".tmp"
	if err := os.WriteFile(temp, data, 0o644); err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "LOGO_STORE_FAILED", "failed to store logo")
		return
	}
	if err := os.Rename(temp, target); err != nil {
		_ = os.Remove(temp)
		s.writeError(w, http.StatusInternalServerError, requestID, "LOGO_STORE_FAILED", "failed to store logo")
		return
	}
	current, err := s.store.Agent(tenant.AgentID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load tenant configuration")
		return
	}
	logoPath := "/branding/logos/" + name
	updated, err := s.store.UpdateBranding(tenant.AgentID, current.Name, current.SiteName, logoPath, current.DocURL, current.ContactInfo, current.APIBaseURL, current.AgentHomeSettings)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to save uploaded logo")
		return
	}
	_ = contentType
	s.writeData(w, http.StatusOK, requestID, agentBrandingData(updated))
}

func (s *Server) handleBrandLogoFile(w http.ResponseWriter, r *http.Request, requestID string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/branding/logos/")
	if !regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}\.(png|jpg|webp|gif)$`).MatchString(name) {
		s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "logo not found")
		return
	}
	file := filepath.Join(filepath.Dir(s.cfg.DatabasePath), "logos", name)
	data, err := os.ReadFile(file)
	if err != nil {
		s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "logo not found")
		return
	}
	ext, contentType, ok := detectLogoType(data)
	if !ok || !strings.HasSuffix(name, ext) {
		s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "logo not found")
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(data)
	}
}

func detectLogoType(data []byte) (string, string, bool) {
	switch {
	case bytes.HasPrefix(data, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}):
		return ".png", "image/png", true
	case len(data) >= 3 && bytes.Equal(data[:3], []byte{0xff, 0xd8, 0xff}):
		return ".jpg", "image/jpeg", true
	case bytes.HasPrefix(data, []byte("GIF87a")) || bytes.HasPrefix(data, []byte("GIF89a")):
		return ".gif", "image/gif", true
	case len(data) >= 12 && bytes.Equal(data[:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return ".webp", "image/webp", true
	default:
		return "", "", false
	}
}
