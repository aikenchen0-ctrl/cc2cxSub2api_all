package app

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"infinite-canvas/backend/internal/kernel"
	"infinite-canvas/backend/internal/model"

	"gorm.io/gorm"
)

const sub2APIRelayChannelID = "sub2api-relay"

// EnsureSub2APIRelayChannel provisions an ordinary system channel when the
// integration environment is configured. It only writes existing tables and
// is therefore compatible with CANVAS_AUTO_MIGRATE=false.
func (s *Service) EnsureSub2APIRelayChannel() error {
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("SUB2API_RELAY_BASE_URL")), "/")
	apiKey := strings.TrimSpace(os.Getenv("SUB2API_RELAY_API_KEY"))
	if baseURL == "" && apiKey == "" {
		return s.disableSub2APIRelayChannel()
	}
	if baseURL == "" || apiKey == "" {
		return errors.New("SUB2API_RELAY_BASE_URL and SUB2API_RELAY_API_KEY must be configured together")
	}
	modelNames := splitCSV(os.Getenv("SUB2API_RELAY_MODELS"))
	allowLocal := strings.EqualFold(strings.TrimSpace(os.Getenv("SUB2API_RELAY_ALLOW_LOCAL")), "true")
	if len(modelNames) == 0 {
		modelNames = []string{strings.TrimSpace(os.Getenv("SUB2API_RELAY_MODEL"))}
	}
	if strings.TrimSpace(modelNames[0]) == "" {
		modelNames[0] = "gpt-5.5"
	}
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
	desired := make(map[string]bool, len(modelNames))
	for _, name := range modelNames {
		desired[strings.TrimPrefix(strings.TrimSpace(name), "models/")] = true
	}
	for _, item := range existing {
		name := strings.TrimPrefix(strings.TrimSpace(item.ModelKey), "models/")
		known[name] = true
		if !desired[name] && item.Enabled {
			item.Enabled = false
			item.UpdatedAt = time.Now()
			if err := s.repo.SaveChannelModel(&item); err != nil {
				return err
			}
		}
	}
	for _, name := range modelNames {
		name = strings.TrimPrefix(strings.TrimSpace(name), "models/")
		if name == "" || known[name] {
			continue
		}
		id, err := s.repo.NextPrefixedID("MODEL")
		if err != nil {
			return err
		}
		item := &model.ChannelModel{ID: id, ChannelID: sub2APIRelayChannelID, ModelKey: name, ProviderModelKey: name, DisplayName: name, Capability: "text", Protocol: model.ChannelInterfaceChatCompletion, BillingMode: "fixed_request", PriceConfigured: true, Enabled: true, PriceVersion: 1, UnitPriceMicrocredits: 0, CreatedAt: time.Now(), UpdatedAt: time.Now()}
		if err := s.prepareSub2APIModel(item, name); err != nil {
			return err
		}
		if err := s.repo.Create(item); err != nil {
			return err
		}
		if err := s.ensureSub2APIPriceTier(item); err != nil {
			return err
		}
	}
	for index := range existing {
		name := strings.TrimPrefix(strings.TrimSpace(existing[index].ModelKey), "models/")
		if !desired[name] {
			continue
		}
		if err := s.prepareSub2APIModel(&existing[index], name); err != nil {
			return err
		}
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

func (s *Service) prepareSub2APIModel(item *model.ChannelModel, name string) error {
	item.ModelKey = name
	item.ProviderModelKey = name
	item.DisplayName = name
	item.Capability = "text"
	item.Protocol = model.ChannelInterfaceChatCompletion
	item.BillingMode = "fixed_request"
	item.PriceConfigured = true
	item.Enabled = true
	if item.PriceVersion < 1 {
		item.PriceVersion = 1
	}
	encoded, err := json.Marshal(DefaultModelCapabilityConfigForModel(string(model.ChannelInterfaceChatCompletion), name))
	if err != nil {
		return err
	}
	item.CapabilityConfigJSON = string(encoded)
	item.UpdatedAt = time.Now()
	return nil
}

func (s *Service) ensureSub2APIPriceTier(item *model.ChannelModel) error {
	if len(item.PriceTiers) > 0 {
		return nil
	}
	id, err := s.repo.NextPrefixedID("PTIER")
	if err != nil {
		return err
	}
	return s.repo.Create(&model.ChannelModelPriceTier{
		ID: id, ChannelModelID: item.ID, SelectorKey: "{}", SelectorJSON: "{}", Resolution: "*",
		ProviderModelKey: item.ProviderModelKey, BillingMode: "fixed_request", PriceConfigured: true,
		Enabled: true, PriceVersion: 1, UnitPriceMicrocredits: 0, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
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
