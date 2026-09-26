package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/tigerowo/infinite-canvas/model"
	"github.com/tigerowo/infinite-canvas/repository"
)

const sub2APIRelayChannelID = "sub2api-relay"

// FetchSub2APIUserBalance reads the current Sub2API account balance through
// the managed server-side satellite credential. It never accepts a browser
// supplied credential or subject.
func FetchSub2APIUserBalance(ctx context.Context, subject string) (float64, int, error) {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return 0, http.StatusUnauthorized, errors.New("Sub2API subject is unavailable")
	}
	credential := strings.TrimSpace(os.Getenv("SUB2API_APP_CREDENTIAL"))
	if credential == "" {
		return 0, http.StatusServiceUnavailable, errors.New("Sub2API relay credential is unavailable")
	}
	baseURL, err := sub2APIRelayBaseURL()
	if err != nil {
		return 0, http.StatusServiceUnavailable, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/sub2api/balance", nil)
	if err != nil {
		return 0, http.StatusServiceUnavailable, err
	}
	if err := ApplySub2APIHeadersForUser(request.Header, subject, ""); err != nil {
		return 0, http.StatusServiceUnavailable, err
	}
	client := &http.Client{Timeout: 12 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return 0, http.StatusBadGateway, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusServiceUnavailable {
			return 0, response.StatusCode, fmt.Errorf("Sub2API balance request returned %d", response.StatusCode)
		}
		return 0, http.StatusBadGateway, fmt.Errorf("Sub2API balance request returned %d", response.StatusCode)
	}
	var payload struct {
		Balance float64 `json:"balance"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload); err != nil {
		return 0, http.StatusBadGateway, errors.New("invalid Sub2API balance response")
	}
	return payload.Balance, http.StatusOK, nil
}

func sub2APIRelayBaseURL() (string, error) {
	raw := strings.TrimSpace(os.Getenv("SUB2API_RELAY_BASE_URL"))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("LINK"))
	}
	if raw == "" {
		return "", errors.New("SUB2API_RELAY_BASE_URL or LINK must be configured")
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return "", errors.New("Sub2API relay base URL must be an HTTP(S) origin or /v1 URL")
	}
	path := strings.TrimRight(parsed.Path, "/")
	if path != "" && path != "/v1" {
		return "", errors.New("Sub2API relay base URL must use /v1")
	}
	parsed.Path = "/v1"
	parsed.RawPath = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

// Sub2APIPurchaseURL is intentionally derived from the public LINK only. A
// docker-internal relay URL is not safe to expose as a browser recharge link.
func Sub2APIPurchaseURL() string {
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

func IsSub2APIChannel(channel model.ModelChannel) bool {
	return channel.ID == sub2APIRelayChannelID
}

// SelectSub2APIRelayChannelForModel returns the managed server-side relay.
// It is used for ordinary users so browser-supplied channel IDs cannot bypass
// the shared Sub2API billing and access policy.
func SelectSub2APIRelayChannelForModel(modelName string) (model.ModelChannel, error) {
	return SelectModelChannelForModel(modelName, sub2APIRelayChannelID)
}

func ApplySub2APIHeaders(header http.Header, apiKey string) error {
	return ApplySub2APIHeadersForUser(header, "", apiKey)
}

func ApplySub2APIHeadersForUser(header http.Header, onBehalfOf, apiKey string) error {
	if header == nil {
		return errors.New("Sub2API relay request headers are unavailable")
	}
	credential := strings.TrimSpace(os.Getenv("SUB2API_APP_CREDENTIAL"))
	// Relay calls are always managed satellite calls. Never fall back to a
	// channel-stored API key (which could be a SuperKey) or an anonymous user.
	if credential == "" || strings.TrimSpace(onBehalfOf) == "" {
		header.Del("Authorization")
		header.Del("X-Sub2API-On-Behalf-Of")
		header.Del("X-Sub2API-Satellite")
		return errors.New("Sub2API relay credential or current session subject is unavailable")
	}
	header.Set("Authorization", "Bearer "+credential)
	header.Set("X-Sub2API-On-Behalf-Of", strings.TrimSpace(onBehalfOf))
	header.Set("X-Sub2API-Satellite", "canvas")
	return nil
}

func OverlayUserSub2APIKey(channel model.ModelChannel, userID string) model.ModelChannel {
	if !IsSub2APIChannel(channel) || strings.TrimSpace(userID) == "" {
		return channel
	}
	user, ok, err := repository.GetUserByID(userID)
	if err != nil || !ok {
		return channel
	}
	if subject := strings.TrimSpace(user.Sub2APISubject); subject != "" {
		channel.OnBehalfOf = subject
	}
	return channel
}

func Sub2APIRelayEnabled() bool {
	settings, err := repository.GetSettings()
	if err != nil {
		return false
	}
	for _, channel := range normalizePrivateSetting(settings.Private).Channels {
		if IsSub2APIChannel(channel) && channel.Enabled && channel.BaseURL != "" && channel.APIKey != "" {
			return true
		}
	}
	return false
}

// Validate everything before writing; restarting also rotates keys and retires removed models.
func EnsureSub2APIRelayChannel() error {
	channel, err := configuredSub2APIChannel()
	if err != nil {
		return err
	}
	settings, err := repository.GetSettings()
	if err != nil {
		return err
	}
	previousModels := map[string]bool{}
	found := false
	for i := range settings.Private.Channels {
		if !IsSub2APIChannel(settings.Private.Channels[i]) {
			continue
		}
		for _, name := range settings.Private.Channels[i].Models {
			previousModels[name] = true
		}
		settings.Private.Channels[i] = channel
		found = true
	}
	if !found && !channel.Enabled {
		return nil
	}
	if !found {
		settings.Private.Channels = append(settings.Private.Channels, channel)
	}
	available := []string{}
	for _, name := range settings.Public.ModelChannel.AvailableModels {
		if !previousModels[name] {
			available = append(available, name)
		}
	}
	// Keep a shared model available if another administrator-managed channel still exposes it.
	for _, item := range settings.Private.Channels {
		if item.Enabled && !IsSub2APIChannel(item) {
			for _, name := range item.Models {
				if previousModels[name] {
					available = append(available, name)
				}
			}
		}
	}
	settings.Public.ModelChannel.AvailableModels = uniqueModelNames(append(available, channel.Models...))
	if channel.Enabled {
		enabled := true
		settings.Public.ModelChannel.AllowUserRemoteChannel = &enabled
	}
	// Bypass the admin form's blank-secret preservation when removing the managed key.
	_, err = repository.SaveSettings(normalizeSettings(settings), now())
	return err
}

func configuredSub2APIChannel() (model.ModelChannel, error) {
	channel := model.ModelChannel{ID: sub2APIRelayChannelID, Protocol: "openai", Name: "Sub2API", Weight: 1, Timeout: 600}
	baseURL := strings.TrimSpace(os.Getenv("SUB2API_RELAY_BASE_URL"))
	if baseURL == "" {
		baseURL = strings.TrimSpace(os.Getenv("LINK"))
	}
	if baseURL != "" && !strings.Contains(baseURL, "://") {
		baseURL = "http://" + baseURL
	}
	credential := strings.TrimSpace(os.Getenv("SUB2API_APP_CREDENTIAL"))
	managed := credential != "" || strings.TrimSpace(os.Getenv("SUB2API_SSO_SECRET")) != ""
	if baseURL == "" && credential == "" && !managed {
		return channel, nil
	}
	if baseURL == "" {
		return channel, errors.New("SUB2API_RELAY_BASE_URL or LINK must be configured")
	}
	if credential == "" {
		return channel, errors.New("SUB2API_APP_CREDENTIAL must be configured for managed relay")
	}
	if strings.IndexFunc(credential, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return channel, errors.New("Sub2API relay credential contains whitespace or control characters")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || parsed.RawPath != "" {
		return channel, errors.New("SUB2API_RELAY_BASE_URL must be an HTTP(S) origin or /v1 URL without credentials, query, or fragment")
	}
	if port := parsed.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return channel, errors.New("invalid Sub2API port")
		}
	} else if strings.HasSuffix(parsed.Host, ":") {
		return channel, errors.New("empty Sub2API port")
	}
	if path := strings.TrimRight(parsed.Path, "/"); path != "" && path != "/v1" {
		return channel, errors.New("SUB2API_RELAY_BASE_URL must use /v1, not /api/v1 or a model endpoint")
	}
	parsed.Path = "/v1"
	seen := map[string]string{}
	for _, name := range []string{"SUB2API_RELAY_MODELS", "SUB2API_RELAY_IMAGE_MODELS", "SUB2API_RELAY_VIDEO_MODELS"} {
		raw := os.Getenv(name)
		if strings.TrimSpace(raw) == "" && strings.TrimSpace(os.Getenv("SUB2API_APP_CREDENTIAL")) != "" {
			switch name {
			case "SUB2API_RELAY_MODELS":
				raw = "gpt-5.5"
			case "SUB2API_RELAY_IMAGE_MODELS":
				raw = "gpt-image-2,grok-imagine-image-1.5"
			case "SUB2API_RELAY_VIDEO_MODELS":
				raw = "grok-imagine-video-1.5,seedance-2.0,kling-v3"
			}
		} else if name == "SUB2API_RELAY_MODELS" && strings.TrimSpace(raw) == "" {
			raw = "gpt-5.5"
		}
		for _, item := range strings.Split(raw, ",") {
			if strings.TrimSpace(item) == "" {
				continue
			}
			item = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(item), "models/"))
			if item == "" || utf8.RuneCountInString(item) > 120 || strings.IndexFunc(item, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
				return channel, fmt.Errorf("%s contains an invalid model name", name)
			}
			if previous, ok := seen[item]; ok {
				if previous != name {
					return channel, fmt.Errorf("model %q has conflicting capabilities", item)
				}
				continue
			}
			seen[item] = name
			channel.Models = append(channel.Models, item)
		}
	}
	channel.BaseURL, channel.APIKey, channel.Enabled = parsed.String(), credential, true
	return channel, nil
}
