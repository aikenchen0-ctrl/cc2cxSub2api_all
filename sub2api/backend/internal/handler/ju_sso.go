package handler

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode"
)

type juSSOTicket struct {
	Issuer             string `json:"iss,omitempty"`
	Audience           string `json:"aud,omitempty"`
	Subject            string `json:"sub"`
	Email              string `json:"email,omitempty"`
	Username           string `json:"username,omitempty"`
	DisplayName        string `json:"displayName,omitempty"`
	AvatarURL          string `json:"avatarUrl,omitempty"`
	IssuedAt           int64  `json:"iat"`
	ExpiresAt          int64  `json:"exp"`
	Nonce     string `json:"jti"`
	Next      string `json:"next,omitempty"`
}

func signJuTicket(payload juSSOTicket, secret string) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(body)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(encoded))
	return encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func randomNonce() string {
	var b [18]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconvInt64(time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func strconvInt64(value int64) string {
	const digits = "0123456789"
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	var buf [24]byte
	i := len(buf)
	for value > 0 {
		i--
		buf[i] = digits[value%10]
		value /= 10
	}
	if negative {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func safeJuNext(raw string) string {
	if len(raw) > 2048 || strings.IndexFunc(raw, unicode.IsControl) >= 0 {
		return "/projects"
	}
	raw = strings.TrimSpace(raw)
	if raw == "" || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") || strings.Contains(raw, "\\") || strings.ContainsAny(raw, "\r\n") {
		return "/projects"
	}
	return raw
}

func parseJuCallback(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	local := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"
	if u.Hostname() == "" || (u.Scheme != "https" && !(u.Scheme == "http" && local)) || u.User != nil || u.Opaque != "" || u.Fragment != "" || u.RawQuery != "" || u.ForceQuery || u.Path != "/api/auth/sso/callback" || u.RawPath != "" {
		return nil, fmt.Errorf("invalid Ju SSO callback")
	}
	return u, nil
}

// Revoke Ju sessions through the registered server-to-server endpoint. A
// failed delivery must not be reported as a successful global logout.
func revokeJuSessions(ctx context.Context, userID int64) error {
	callback := strings.TrimSpace(os.Getenv("JU_SSO_CALLBACK_URL"))
	if callback == "" {
		return nil
	}
	target, err := parseJuCallback(callback)
	if err != nil {
		return err
	}
	secret := strings.TrimSpace(os.Getenv("SUB2API_SSO_SECRET"))
	if len(secret) < 32 {
		return fmt.Errorf("Ju SSO is not configured")
	}
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return err
	}
	now := time.Now()
	ticket, err := signJuTicket(juSSOTicket{Issuer: "sub2api", Audience: "ju:logout", Subject: strconvInt64(userID), IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Minute).Unix(), Nonce: hex.EncodeToString(nonce[:])}, secret)
	if err != nil {
		return err
	}
	body, err := json.Marshal(map[string]string{"ticket": ticket})
	if err != nil {
		return err
	}
	target.Path = "/api/auth/sso/logout"
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("unable to construct Ju logout request")
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Ju logout delivery failed")
	}
	defer resp.Body.Close()
	var result struct {
		Code int `json:"code"`
		Data struct {
			Revoked bool `json:"revoked"`
		} `json:"data"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&result) != nil || result.Code != 0 || !result.Data.Revoked {
		return fmt.Errorf("Ju logout was not acknowledged")
	}
	return nil
}
