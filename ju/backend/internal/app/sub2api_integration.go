package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"infinite-canvas/backend/internal/auth"
	"infinite-canvas/backend/internal/kernel"
	"infinite-canvas/backend/internal/model"

	"gorm.io/gorm"
)

const sub2APIRelayChannelID = "sub2api-relay"

// Sub2APIOnBehalfOf returns the Sub2API user id for a ju user. Memory is a
// cache; the durable mapping lives on the sub2api identity row.
func (s *Service) Sub2APIOnBehalfOf(userID string) string {
	if key := auth.UserRelayKey(userID); key != "" {
		return key
	}
	identity, err := s.repo.UserIdentityForUser(userID, "sub2api")
	if err != nil || identity == nil {
		return ""
	}
	subject := strings.TrimSpace(identity.Subject)
	if subject != "" {
		auth.StoreUserRelayKey(userID, subject)
	}
	return subject
}

type sub2APIRelayModel struct {
	name       string
	capability string
	protocol   model.ChannelInterfaceType
	configJSON string
}

// EnsureSub2APIRelayChannel provisions an ordinary system channel when the
// integration environment is configured. It only writes existing tables and
// is therefore compatible with CANVAS_AUTO_MIGRATE=false.
func (s *Service) EnsureSub2APIRelayChannel() error {
	baseURL := strings.TrimSpace(os.Getenv("SUB2API_RELAY_BASE_URL"))
	if baseURL == "" {
		baseURL = strings.TrimSpace(os.Getenv("LINK"))
	}
	if baseURL != "" && !strings.Contains(baseURL, "://") {
		baseURL = "http://" + baseURL
	}
	apiKey := strings.TrimSpace(os.Getenv("SUB2API_RELAY_API_KEY"))
	credential := strings.TrimSpace(os.Getenv("SUB2API_APP_CREDENTIAL"))
	if baseURL == "" && apiKey == "" && credential == "" {
		return s.disableSub2APIRelayChannel()
	}
	if baseURL == "" {
		return errors.New("SUB2API_RELAY_BASE_URL or LINK must be configured")
	}
	if apiKey == "" {
		apiKey = credential
	}
	if apiKey == "" {
		return errors.New("SUB2API_APP_CREDENTIAL or SUB2API_RELAY_API_KEY must be configured")
	}
	baseURL, err := normalizeSub2APIBaseURL(baseURL)
	if err != nil {
		return err
	}
	models, err := configuredSub2APIModels()
	if err != nil {
		return err
	}
	modelNames := make([]string, len(models))
	desired := make(map[string]sub2APIRelayModel, len(models))
	for index, item := range models {
		modelNames[index], desired[item.name] = item.name, item
	}
	allowLocal := strings.EqualFold(strings.TrimSpace(os.Getenv("SUB2API_RELAY_ALLOW_LOCAL")), "true")
	channel, err := s.repo.AdminSystemChannel(sub2APIRelayChannelID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		channel = &model.ModelChannel{ID: sub2APIRelayChannelID, Scope: model.ChannelScopeSystem, Enabled: true, Name: "Sub2API 上游", PublicAlias: "智能剧场", SortOrder: -100, BaseURL: baseURL, APIFormat: "openai", ConcurrencyLimit: 2, CreatedAt: time.Now(), UpdatedAt: time.Now()}
		channel.AllowLocalChannel = allowLocal
		channel.APIKey = apiKey
		if err := s.encryptSystemChannelSecrets(channel); err != nil {
			return err
		}
		if encoded, marshalErr := json.Marshal(modelNames); marshalErr == nil {
			channel.ModelsJSON = string(encoded)
		}
		if err := s.repo.Create(channel); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		if err := s.decryptSystemChannelSecrets(channel); err != nil {
			return err
		}
		encodedModels, _ := json.Marshal(modelNames)
		changed := channel.BaseURL != baseURL || channel.APIKey != apiKey || channel.AllowLocalChannel != allowLocal || !channel.Enabled || channel.ModelsJSON != string(encodedModels)
		channel.BaseURL, channel.APIKey, channel.AllowLocalChannel, channel.Enabled, channel.UpdatedAt = baseURL, apiKey, allowLocal, true, time.Now()
		if encoded, marshalErr := json.Marshal(modelNames); marshalErr == nil {
			channel.ModelsJSON = string(encoded)
		}
		if changed {
			if err := s.encryptSystemChannelSecrets(channel); err != nil {
				return err
			}
			if err := s.repo.Save(channel); err != nil {
				return err
			}
		}
	}

	existing, err := s.repo.ChannelModels(sub2APIRelayChannelID, true)
	if err != nil {
		return err
	}
	known := make(map[string]bool, len(existing))
	for _, item := range existing {
		name := strings.TrimPrefix(strings.TrimSpace(item.ModelKey), "models/")
		known[name] = true
		if _, wanted := desired[name]; !wanted && item.Enabled {
			item.Enabled = false
			item.UpdatedAt = time.Now()
			if err := s.repo.SaveChannelModel(&item); err != nil {
				return err
			}
		}
	}
	for _, configured := range models {
		if known[configured.name] {
			continue
		}
		id, err := s.repo.NextPrefixedID("MODEL")
		if err != nil {
			return err
		}
		item := &model.ChannelModel{ID: id, ChannelID: sub2APIRelayChannelID, BillingMode: "fixed_request", PriceConfigured: true, Enabled: true, PriceVersion: 1, UnitPriceMicrocredits: 0, CreatedAt: time.Now(), UpdatedAt: time.Now()}
		prepareSub2APIModel(item, configured)
		if err := s.repo.Create(item); err != nil {
			return err
		}
		if err := s.ensureSub2APIPriceTier(item); err != nil {
			return err
		}
	}
	for index := range existing {
		name := strings.TrimPrefix(strings.TrimSpace(existing[index].ModelKey), "models/")
		configured, wanted := desired[name]
		if !wanted {
			continue
		}
		prepareSub2APIModel(&existing[index], configured)
		if err := s.repo.SaveChannelModel(&existing[index]); err != nil {
			return err
		}
		if err := s.ensureSub2APIPriceTier(&existing[index]); err != nil {
			return err
		}
	}
	return nil
}

// disableSub2APIRelayChannel removes the server-side secret and disables the
// existing rows when integration settings are cleared. It deliberately keeps
// the records for audit/history and does not alter the database schema.
func (s *Service) disableSub2APIRelayChannel() error {
	channel, err := s.repo.AdminSystemChannel(sub2APIRelayChannelID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := s.decryptSystemChannelSecrets(channel); err != nil {
		return err
	}
	channel.APIKey = ""
	channel.SecretKey = ""
	channel.BaseURL = ""
	channel.Enabled = false
	channel.UpdatedAt = time.Now()
	if err := s.encryptSystemChannelSecrets(channel); err != nil {
		return err
	}
	if err := s.repo.Save(channel); err != nil {
		return err
	}
	items, err := s.repo.ChannelModels(sub2APIRelayChannelID, true)
	if err != nil {
		return err
	}
	for index := range items {
		if !items[index].Enabled {
			continue
		}
		items[index].Enabled = false
		items[index].UpdatedAt = time.Now()
		if err := s.repo.SaveChannelModel(&items[index]); err != nil {
			return err
		}
	}
	return nil
}

func prepareSub2APIModel(item *model.ChannelModel, configured sub2APIRelayModel) {
	item.ModelKey = configured.name
	item.ProviderModelKey = configured.name
	if strings.TrimSpace(item.DisplayName) == "" {
		item.DisplayName = configured.name
	}
	item.Capability = configured.capability
	item.Protocol = configured.protocol
	item.Enabled = true
	if item.PriceVersion < 1 {
		item.PriceVersion = 1
	}
	if item.CapabilityConfigJSON != configured.configJSON {
		item.CapabilityConfigJSON = configured.configJSON
		item.CapabilityVersion++
	}
	item.UpdatedAt = time.Now()
}

func (s *Service) ensureSub2APIPriceTier(item *model.ChannelModel) error {
	for _, tier := range item.PriceTiers {
		// Repository reads can synthesize an ID-less legacy tier. Persist a
		// real row so browser quotes and task billing share the same tier ID.
		if strings.TrimSpace(tier.ID) != "" {
			return nil
		}
	}
	id, err := s.repo.NextPrefixedID("PTIER")
	if err != nil {
		return err
	}
	return s.repo.Create(&model.ChannelModelPriceTier{
		ID: id, ChannelModelID: item.ID, SelectorKey: "{}", SelectorJSON: "{}", Resolution: "*",
		ProviderModelKey: item.ProviderModelKey, BillingMode: item.BillingMode, PriceConfigured: item.PriceConfigured,
		Enabled: true, PriceVersion: item.PriceVersion, UnitPriceMicrocredits: item.UnitPriceMicrocredits,
		InputTokenPriceMicrocredits: item.InputTokenPriceMicrocredits, OutputTokenPriceMicrocredits: item.OutputTokenPriceMicrocredits,
		CachedTokenPriceMicrocredits: item.CachedTokenPriceMicrocredits, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
}

func normalizeSub2APIBaseURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed == nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || parsed.RawPath != "" {
		return "", errors.New("SUB2API_RELAY_BASE_URL must be an HTTP(S) origin or /v1 URL without credentials, query, or fragment")
	}
	if port := parsed.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return "", errors.New("SUB2API_RELAY_BASE_URL has an invalid port")
		}
	} else if strings.HasSuffix(parsed.Host, ":") {
		return "", errors.New("SUB2API_RELAY_BASE_URL has an empty port")
	}
	// /api/v1 is Sub2API's account API, not its model gateway.
	if path := strings.TrimRight(parsed.Path, "/"); path != "" && path != "/v1" {
		return "", errors.New("SUB2API_RELAY_BASE_URL must use the Sub2API /v1 model gateway, not /api/v1 or a model endpoint")
	}
	parsed.Path = "/v1"
	return parsed.String(), nil
}

func configuredSub2APIModels() ([]sub2APIRelayModel, error) {
	textModels := splitCSV(os.Getenv("SUB2API_RELAY_MODELS"))
	if len(textModels) == 0 {
		textModels = []string{firstNonEmpty(strings.TrimSpace(os.Getenv("SUB2API_RELAY_MODEL")), "gpt-5.5")}
	}
	imageModels := splitCSV(os.Getenv("SUB2API_RELAY_IMAGE_MODELS"))
	videoModels := splitCSV(os.Getenv("SUB2API_RELAY_VIDEO_MODELS"))
	if strings.TrimSpace(os.Getenv("SUB2API_APP_CREDENTIAL")) != "" {
		if len(imageModels) == 0 {
			imageModels = []string{"gpt-image-2"}
		}
		if len(videoModels) == 0 {
			videoModels = []string{"grok-imagine-video-1.5", "seedance-2.0", "kling-v3"}
		}
	}
	groups := []struct {
		env        string
		capability string
		protocol   model.ChannelInterfaceType
		names      []string
	}{
		{"SUB2API_RELAY_MODELS", "text", model.ChannelInterfaceChatCompletion, textModels},
		{"SUB2API_RELAY_IMAGE_MODELS", "image", model.ChannelInterfaceOpenAIImage, imageModels},
		{"SUB2API_RELAY_VIDEO_MODELS", "video", model.ChannelInterfaceType("sub2api-video-v1"), videoModels},
	}
	items := make([]sub2APIRelayModel, 0)
	seen := make(map[string]string)
	for _, group := range groups {
		for _, raw := range group.names {
			name := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "models/"))
			if name == "" || utf8.RuneCountInString(name) > 120 || strings.IndexFunc(name, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
				return nil, fmt.Errorf("%s contains an invalid model name", group.env)
			}
			if previous, exists := seen[name]; exists {
				if previous != group.capability {
					return nil, fmt.Errorf("Sub2API model %q is configured for both %s and %s; each model name must have one capability", name, previous, group.capability)
				}
				continue
			}
			protocol := group.protocol
			if group.capability == "image" {
				protocol = sub2APIImageProtocol(name)
			}
			config := DefaultModelCapabilityConfigForModel(string(protocol), name)
			if group.capability == "video" {
				var err error
				config, err = sub2APIVideoCapabilityConfig(name)
				if err != nil {
					return nil, err
				}
			}
			encoded, err := json.Marshal(config)
			if err != nil {
				return nil, err
			}
			seen[name] = group.capability
			items = append(items, sub2APIRelayModel{name: name, capability: group.capability, protocol: protocol, configJSON: string(encoded)})
		}
	}
	return items, nil
}

func splitCSV(raw string) []string {
	values := make([]string, 0)
	for _, value := range strings.Split(raw, ",") {
		value = strings.TrimSpace(value)
		if value != "" {
			values = append(values, value)
		}
	}
	return kernel.UniqueNonEmpty(values)
}
