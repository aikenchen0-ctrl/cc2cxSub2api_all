package service

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/tigerowo/infinite-canvas/model"
	"github.com/tigerowo/infinite-canvas/repository"
)

const sub2APIRelayChannelID = "sub2api-relay"

func IsSub2APIChannel(channel model.ModelChannel) bool {
	return channel.ID == sub2APIRelayChannelID
}

// SelectSub2APIRelayChannelForModel returns the managed server-side relay.
// It is used for ordinary users so browser-supplied channel IDs cannot bypass
// the shared Sub2API billing and access policy.
func SelectSub2APIRelayChannelForModel(modelName string) (model.ModelChannel, error) {
	return SelectModelChannelForModel(modelName, sub2APIRelayChannelID)
}

func ApplySub2APIHeaders(header http.Header, apiKey string) {
	ApplySub2APIHeadersForUser(header, "", apiKey)
}

func ApplySub2APIHeadersForUser(header http.Header, onBehalfOf, apiKey string) {
	credential := strings.TrimSpace(os.Getenv("SUB2API_APP_CREDENTIAL"))
	if strings.TrimSpace(onBehalfOf) != "" && credential != "" {
		header.Set("Authorization", "Bearer "+credential)
		header.Set("X-Sub2API-On-Behalf-Of", strings.TrimSpace(onBehalfOf))
		header.Set("X-Sub2API-Satellite", "canvas")
		return
	}
	header.Set("Authorization", "Bearer "+apiKey)
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
	key := strings.TrimSpace(os.Getenv("SUB2API_RELAY_API_KEY"))
	credential := strings.TrimSpace(os.Getenv("SUB2API_APP_CREDENTIAL"))
	if baseURL == "" && key == "" && credential == "" {
		return channel, nil
	}
	if baseURL == "" {
		return channel, errors.New("SUB2API_RELAY_BASE_URL or LINK must be configured")
	}
	if key == "" {
		key = credential
	}
	if key == "" {
		return channel, errors.New("SUB2API_APP_CREDENTIAL or SUB2API_RELAY_API_KEY must be configured")
	}
	if strings.IndexFunc(key, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return channel, errors.New("SUB2API_RELAY_API_KEY contains whitespace or control characters")
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
	channel.BaseURL, channel.APIKey, channel.Enabled = parsed.String(), key, true
	return channel, nil
}
