package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// MainClient is the only component allowed to talk to the Sub2API process. It
// deliberately has no database access and keeps the administrator key in the
// process environment, so it can never be serialized into a browser response.
type MainClient struct {
	cfg        Config
	http       *http.Client
	streamHTTP *http.Client
}

type MainAPIError struct {
	Status  int
	Code    string
	Message string
	Body    []byte
}

func (e *MainAPIError) Error() string {
	if e == nil {
		return "main api error"
	}
	if e.Code != "" {
		return fmt.Sprintf("main api error: %s (%d): %s", e.Code, e.Status, e.Message)
	}
	return fmt.Sprintf("main api error (%d): %s", e.Status, e.Message)
}

type mainEnvelope struct {
	Code     json.RawMessage `json:"code"`
	Message  string          `json:"message"`
	Reason   string          `json:"reason"`
	Metadata map[string]any  `json:"metadata"`
	Data     json.RawMessage `json:"data"`
}

type MainAuthResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
	User         json.RawMessage
	Requires2FA  bool
	TempToken    string
	RawData      json.RawMessage
}

type MainUserResult struct {
	ID      string
	Email   string
	Balance int64
	Raw     json.RawMessage
}

type MainUsageResult struct {
	ID          string
	RequestID   string
	TotalCents  int64
	ActualCents int64
	Model       string
	Raw         json.RawMessage
}

func NewMainClient(cfg Config) *MainClient {
	return &MainClient{
		cfg:        cfg,
		http:       &http.Client{Timeout: cfg.MainRequestTimeout},
		streamHTTP: &http.Client{Timeout: cfg.ModelStreamTimeout},
	}
}

func (c *MainClient) endpoint(path string) string {
	return strings.TrimRight(c.cfg.MainAPIBaseURL, "/") + "/" + strings.TrimLeft(path, "/")
}

func (c *MainClient) request(ctx context.Context, method, target string, body []byte, headers http.Header) (*http.Response, []byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if readErr != nil {
		return resp, nil, readErr
	}
	return resp, data, nil
}

func (c *MainClient) jsonRequest(ctx context.Context, method, path string, payload any, token string, admin bool) (json.RawMessage, error) {
	var body []byte
	var err error
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return nil, err
		}
	}
	headers := make(http.Header)
	headers.Set("Accept", "application/json")
	if payload != nil {
		headers.Set("Content-Type", "application/json")
	}
	if token != "" {
		headers.Set("Authorization", "Bearer "+token)
	}
	if admin {
		if strings.TrimSpace(c.cfg.AdminKey) == "" {
			return nil, &MainAPIError{Status: http.StatusServiceUnavailable, Code: "MAIN_ADMIN_KEY_MISSING", Message: "main admin key is not configured"}
		}
		headers.Set("x-api-key", c.cfg.AdminKey)
	}
	resp, data, err := c.request(ctx, method, c.endpoint(path), body, headers)
	if err != nil {
		return nil, err
	}
	return unwrapMainResponse(resp.StatusCode, data)
}

func unwrapMainResponse(status int, body []byte) (json.RawMessage, error) {
	var envelope mainEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		if status >= 200 && status < 300 {
			return json.RawMessage(body), nil
		}
		return nil, &MainAPIError{Status: status, Message: http.StatusText(status), Body: body}
	}
	if status < 200 || status >= 300 || (len(envelope.Code) > 0 && string(envelope.Code) != "0" && string(envelope.Code) != "null") {
		code := ""
		if envelope.Reason != "" {
			code = envelope.Reason
		} else if len(envelope.Code) > 0 {
			code = strings.Trim(string(envelope.Code), `"`)
		}
		message := strings.TrimSpace(envelope.Message)
		if message == "" {
			message = http.StatusText(status)
		}
		return nil, &MainAPIError{Status: status, Code: code, Message: message, Body: body}
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return json.RawMessage(`{}`), nil
	}
	return envelope.Data, nil
}

func (c *MainClient) Login(ctx context.Context, email, password string) (MainAuthResult, error) {
	data, err := c.jsonRequest(ctx, http.MethodPost, "/auth/login", map[string]any{
		"email": email, "password": password,
	}, "", false)
	if err != nil {
		return MainAuthResult{}, err
	}
	return decodeAuthResult(data)
}

func (c *MainClient) Register(ctx context.Context, payload map[string]any) (MainAuthResult, error) {
	data, err := c.jsonRequest(ctx, http.MethodPost, "/auth/register", payload, "", false)
	if err != nil {
		return MainAuthResult{}, err
	}
	return decodeAuthResult(data)
}

func (c *MainClient) Login2FA(ctx context.Context, tempToken, code string) (MainAuthResult, error) {
	data, err := c.jsonRequest(ctx, http.MethodPost, "/auth/login/2fa", map[string]any{
		"temp_token": tempToken, "totp_code": code,
	}, "", false)
	if err != nil {
		return MainAuthResult{}, err
	}
	return decodeAuthResult(data)
}

func (c *MainClient) Refresh(ctx context.Context, refreshToken string) (MainAuthResult, error) {
	data, err := c.jsonRequest(ctx, http.MethodPost, "/auth/refresh", map[string]any{
		"refresh_token": refreshToken,
	}, "", false)
	if err != nil {
		return MainAuthResult{}, err
	}
	return decodeAuthResult(data)
}

func (c *MainClient) CurrentUser(ctx context.Context, accessToken string) (json.RawMessage, error) {
	return c.jsonRequest(ctx, http.MethodGet, "/auth/me", nil, accessToken, false)
}

func (c *MainClient) Logout(ctx context.Context, refreshToken string) error {
	if refreshToken == "" {
		return nil
	}
	_, err := c.jsonRequest(ctx, http.MethodPost, "/auth/logout", map[string]any{
		"refresh_token": refreshToken,
	}, "", false)
	return err
}

func (c *MainClient) AdminCreateUser(ctx context.Context, payload map[string]any) (json.RawMessage, error) {
	return c.jsonRequest(ctx, http.MethodPost, "/admin/users", payload, "", true)
}

func (c *MainClient) AdminGetUser(ctx context.Context, mainUserID string) (MainUserResult, error) {
	data, err := c.jsonRequest(ctx, http.MethodGet, "/admin/users/"+url.PathEscape(mainUserID), nil, "", true)
	if err != nil {
		return MainUserResult{}, err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return MainUserResult{}, err
	}
	return MainUserResult{
		ID:      jsonID(raw["id"]),
		Email:   stringValue(raw["email"]),
		Balance: moneyValue(raw["balance"]),
		Raw:     data,
	}, nil
}

func (c *MainClient) AdminUpdateBalance(ctx context.Context, mainUserID string, balance float64, operation, notes string) (json.RawMessage, error) {
	return c.jsonRequest(ctx, http.MethodPost, "/admin/users/"+url.PathEscape(mainUserID)+"/balance", map[string]any{
		"balance": balance, "operation": operation, "notes": notes,
	}, "", true)
}

// AdminFindUsage looks up authoritative main-site usage by the request id that
// AgentAPI sent on the model relay. It intentionally returns only billing
// fields needed for local reconciliation; the raw DTO is retained for audit.
func (c *MainClient) AdminFindUsage(ctx context.Context, requestID string) ([]MainUsageResult, error) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return nil, fmt.Errorf("request id is required")
	}
	path := "/admin/usage?request_id=" + url.QueryEscape(requestID) + "&page=1&page_size=20"
	data, err := c.jsonRequest(ctx, http.MethodGet, path, nil, "", true)
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(data, &envelope); err == nil && envelope.Items != nil {
		return decodeMainUsageItems(envelope.Items), nil
	}
	var items []map[string]any
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	return decodeMainUsageItems(items), nil
}

func decodeMainUsageItems(items []map[string]any) []MainUsageResult {
	result := make([]MainUsageResult, 0, len(items))
	for _, item := range items {
		requestID := stringValue(item["request_id"])
		if requestID == "" {
			continue
		}
		result = append(result, MainUsageResult{
			ID:          jsonID(item["id"]),
			RequestID:   requestID,
			TotalCents:  moneyValue(item["total_cost"]),
			ActualCents: moneyValue(item["actual_cost"]),
			Model:       stringValue(item["model"]),
			Raw:         mustJSON(item),
		})
	}
	return result
}

func mustJSON(value any) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return data
}

// RelayModel creates a request to the main /v1 gateway. It returns a fully
// buffered response so the caller can settle the local ledger after a stream
// finishes and so upstream Set-Cookie headers can never reach the browser.
func (c *MainClient) RelayModel(ctx context.Context, path string, query url.Values, body []byte, mainUserID, requestID string) (int, http.Header, []byte, error) {
	return c.RelayModelMethod(ctx, http.MethodPost, path, query, body, mainUserID, requestID)
}

// RelayModelMethod forwards the client's HTTP method while injecting the
// server-side application credential and on-behalf-of identity. Keeping the
// method parameter here means GET endpoints such as /v1/models and video
// polling work correctly without exposing the upstream credential.
func (c *MainClient) RelayModelMethod(ctx context.Context, method, path string, query url.Values, body []byte, mainUserID, requestID string) (int, http.Header, []byte, error) {
	return c.relayModelMethod(ctx, method, path, query, body, "", mainUserID, requestID)
}

// RelayModelMethodWithContentType is the model relay variant used by the HTTP
// handler when the client sends a multipart request. Keeping the original
// media type (including its boundary) lets image-edit and other upload
// endpoints be parsed by Sub2API; all authentication and identity headers are
// still generated exclusively on the server.
func (c *MainClient) RelayModelMethodWithContentType(ctx context.Context, method, path string, query url.Values, body []byte, contentType, mainUserID, requestID string) (int, http.Header, []byte, error) {
	return c.relayModelMethod(ctx, method, path, query, body, contentType, mainUserID, requestID)
}

func (c *MainClient) relayModelMethod(ctx context.Context, method, path string, query url.Values, body []byte, contentType, mainUserID, requestID string) (int, http.Header, []byte, error) {
	resp, err := c.OpenModelResponse(ctx, method, path, query, body, contentType, mainUserID, requestID)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return resp.StatusCode, filteredResponseHeaders(resp.Header), nil, err
	}
	return resp.StatusCode, filteredResponseHeaders(resp.Header), data, nil
}

// OpenModelResponse starts a model request without buffering its response.
// The caller owns resp.Body and must close it. It is used for explicit
// streaming requests; buffered callers should use RelayModelMethodWithContentType.
func (c *MainClient) OpenModelResponse(ctx context.Context, method, path string, query url.Values, body []byte, contentType, mainUserID, requestID string) (*http.Response, error) {
	target := strings.TrimRight(c.cfg.MainModelBaseURL, "/")
	path = strings.TrimPrefix(path, "/v1")
	if path == "" {
		path = "/"
	}
	target += "/" + strings.TrimLeft(path, "/")
	if encoded := query.Encode(); encoded != "" {
		target += "?" + encoded
	}
	headers := make(http.Header)
	headers.Set("Accept", "application/json, text/event-stream")
	contentType = strings.TrimSpace(contentType)
	if contentType == "" {
		contentType = "application/json"
	}
	headers.Set("Content-Type", contentType)
	if c.cfg.AppCredential == "" {
		return nil, &MainAPIError{Status: http.StatusServiceUnavailable, Code: "MAIN_APP_CREDENTIAL_MISSING", Message: "model relay credential is not configured"}
	}
	// The caller may carry a proxy user's id for local accounting, but the
	// public model gateway must always charge the configured owner account.
	if ownerID := strings.TrimSpace(c.cfg.OwnerMainUserID); ownerID != "" {
		mainUserID = ownerID
	}
	if strings.TrimSpace(mainUserID) == "" {
		return nil, &MainAPIError{Status: http.StatusServiceUnavailable, Code: "MAIN_OWNER_MISSING", Message: "model billing owner is not configured"}
	}
	headers.Set("Authorization", "Bearer "+c.cfg.AppCredential)
	headers.Set("X-Sub2API-On-Behalf-Of", mainUserID)
	headers.Set("X-Sub2API-Satellite", c.cfg.SatelliteSlug)
	if requestID != "" {
		headers.Set("X-Request-ID", requestID)
	}
	if method == "" {
		method = http.MethodPost
	}
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	return c.streamHTTP.Do(req)
}

func filteredResponseHeaders(source http.Header) http.Header {
	result := make(http.Header)
	for key, values := range source {
		lower := strings.ToLower(key)
		if lower == "set-cookie" || lower == "authorization" || strings.HasPrefix(lower, "x-sub2api-") {
			continue
		}
		for _, value := range values {
			result.Add(key, value)
		}
	}
	return result
}

func decodeAuthResult(data json.RawMessage) (MainAuthResult, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return MainAuthResult{}, err
	}
	result := MainAuthResult{RawData: append(json.RawMessage(nil), data...)}
	result.AccessToken = rawString(raw["access_token"])
	result.RefreshToken = rawString(raw["refresh_token"])
	result.ExpiresIn = rawInt(raw["expires_in"])
	result.User = raw["user"]
	result.Requires2FA = rawBool(raw["requires_2fa"])
	result.TempToken = rawString(raw["temp_token"])
	if result.User == nil {
		result.User = json.RawMessage(`{}`)
	}
	return result, nil
}

func rawString(raw json.RawMessage) string {
	var value string
	_ = json.Unmarshal(raw, &value)
	return value
}

func rawInt(raw json.RawMessage) int {
	var value int
	if json.Unmarshal(raw, &value) == nil {
		return value
	}
	var number float64
	if json.Unmarshal(raw, &number) == nil {
		return int(number)
	}
	return 0
}

func rawBool(raw json.RawMessage) bool {
	var value bool
	_ = json.Unmarshal(raw, &value)
	return value
}

func jsonID(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	case json.Number:
		return typed.String()
	case int64:
		return strconv.FormatInt(typed, 10)
	case int:
		return strconv.Itoa(typed)
	default:
		return ""
	}
}

func stringValue(value any) string {
	if typed, ok := value.(string); ok {
		return strings.TrimSpace(typed)
	}
	return ""
}

func moneyValue(value any) int64 {
	switch typed := value.(type) {
	case float64:
		return int64(typed*100 + 0.5)
	case json.Number:
		parsed, _ := strconv.ParseFloat(typed.String(), 64)
		return int64(parsed*100 + 0.5)
	case string:
		parsed, err := parseCents(typed)
		if err == nil {
			return parsed
		}
	}
	return 0
}

func mainUserIDFromJSON(data []byte) string {
	var value map[string]any
	if json.Unmarshal(data, &value) != nil {
		return ""
	}
	for _, key := range []string{"id", "user_id", "uid"} {
		if id := jsonID(value[key]); id != "" {
			return id
		}
	}
	return ""
}

func isTransientMainError(err error) bool {
	var apiErr *MainAPIError
	if errors.As(err, &apiErr) {
		return apiErr.Status == 429 || apiErr.Status >= 500
	}
	return true
}

func contextWithMainTimeout(ctx context.Context, cfg Config) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(ctx, cfg.MainRequestTimeout)
}
