package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Sub2APIUserBalance reads the current user's account balance using the same
// server-side satellite identity as model relay calls.
func (s *Service) Sub2APIUserBalance(ctx context.Context, userID string) (float64, int, error) {
	credential := strings.TrimSpace(os.Getenv("SUB2API_APP_CREDENTIAL"))
	subject := s.Sub2APIOnBehalfOf(userID)
	if subject == "" {
		return 0, http.StatusUnauthorized, errors.New("Sub2API identity is unavailable for this session")
	}
	if credential == "" {
		return 0, http.StatusServiceUnavailable, errors.New("Sub2API satellite credential is unavailable")
	}
	raw := strings.TrimSpace(os.Getenv("SUB2API_RELAY_BASE_URL"))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("LINK"))
	}
	if raw == "" {
		return 0, http.StatusServiceUnavailable, errors.New("Sub2API relay URL is unavailable")
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	baseURL, err := normalizeSub2APIBaseURL(raw)
	if err != nil {
		return 0, http.StatusServiceUnavailable, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/sub2api/balance", nil)
	if err != nil {
		return 0, http.StatusServiceUnavailable, err
	}
	request.Header.Set("Authorization", "Bearer "+credential)
	request.Header.Set("X-Sub2API-On-Behalf-Of", subject)
	request.Header.Set("X-Sub2API-Satellite", "ju")
	response, err := (&http.Client{Timeout: 12 * time.Second}).Do(request)
	if err != nil {
		return 0, http.StatusBadGateway, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusServiceUnavailable {
			return 0, response.StatusCode, errors.New("Sub2API balance request was rejected")
		}
		return 0, http.StatusBadGateway, errors.New("Sub2API balance request failed")
	}
	var payload struct {
		Balance float64 `json:"balance"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload); err != nil {
		return 0, http.StatusBadGateway, errors.New("invalid Sub2API balance response")
	}
	return payload.Balance, http.StatusOK, nil
}

// Sub2APIPurchaseURL only uses public LINK; relay container URLs are not
// suitable for navigation from a user's browser.
func (s *Service) Sub2APIPurchaseURL() string {
	raw := strings.TrimSpace(os.Getenv("LINK"))
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return ""
	}
	parsed.Path = "/purchase"
	parsed.RawPath = ""
	return parsed.String()
}
