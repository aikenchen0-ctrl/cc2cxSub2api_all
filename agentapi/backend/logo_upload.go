package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const maxLogoUploadBytes = 1 << 20
const maxLogoEncodedBytes = ((maxLogoUploadBytes + 2) / 3) * 4
const maxLogoDataURLBytes = maxLogoEncodedBytes + 64

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
	_, contentType, ok := detectLogoType(data)
	if !ok {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_LOGO", "logo must be a PNG, JPEG, WEBP, or GIF image")
		return
	}
	current, err := s.store.Agent(tenant.AgentID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load tenant configuration")
		return
	}
	logo := logoDataURL(data, contentType)
	updated, err := s.store.UpdateBranding(tenant.AgentID, current.Name, current.SiteName, logo, current.DocURL, current.ContactInfo, current.APIBaseURL, current.AgentHomeSettings)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to save uploaded logo")
		return
	}
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

func logoDataURL(data []byte, contentType string) string {
	return "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(data)
}

func decodeLogoDataURL(value string) ([]byte, string, error) {
	if len(value) > maxLogoDataURLBytes {
		return nil, "", fmt.Errorf("embedded logo must be at most 1 MB")
	}
	if !strings.HasPrefix(value, "data:image/") {
		return nil, "", fmt.Errorf("embedded logo must be a supported image data URL")
	}
	comma := strings.IndexByte(value, ',')
	if comma < 0 {
		return nil, "", fmt.Errorf("embedded logo data URL is malformed")
	}
	metadata := value[len("data:"):comma]
	if !strings.HasSuffix(metadata, ";base64") {
		return nil, "", fmt.Errorf("embedded logo must use base64 encoding")
	}
	contentType := strings.TrimSuffix(metadata, ";base64")
	switch contentType {
	case "image/png", "image/jpeg", "image/webp", "image/gif":
	default:
		return nil, "", fmt.Errorf("embedded logo must be PNG, JPEG, WEBP, or GIF")
	}
	encoded := value[comma+1:]
	if encoded == "" {
		return nil, "", fmt.Errorf("embedded logo data URL is malformed")
	}
	for _, r := range encoded {
		if r < 0x20 || r == 0x7f {
			return nil, "", fmt.Errorf("embedded logo data URL is malformed")
		}
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, "", fmt.Errorf("embedded logo base64 is invalid")
	}
	if len(data) == 0 || len(data) > maxLogoUploadBytes {
		return nil, "", fmt.Errorf("embedded logo must be an image of at most 1 MB")
	}
	_, detectedType, ok := detectLogoType(data)
	if !ok || detectedType != contentType {
		return nil, "", fmt.Errorf("embedded logo content does not match its image type")
	}
	return data, contentType, nil
}
