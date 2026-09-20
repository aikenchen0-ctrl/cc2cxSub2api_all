package app

import (
	"encoding/json"
	"strings"
	"testing"

	"infinite-canvas/backend/internal/model"

	"gorm.io/gorm"
)

func newSub2APIIntegrationTestService(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()
	for _, name := range []string{"SUB2API_RELAY_BASE_URL", "SUB2API_APP_CREDENTIAL", "SUB2API_RELAY_MODEL", "SUB2API_RELAY_MODELS", "SUB2API_RELAY_IMAGE_MODELS", "SUB2API_RELAY_VIDEO_MODELS", "SUB2API_RELAY_ALLOW_LOCAL"} {
		t.Setenv(name, "")
	}
	t.Setenv("SUB2API_RELAY_BASE_URL", "http://sub2api:8080/v1")
	t.Setenv("SUB2API_APP_CREDENTIAL", "satellite-test-credential")
	svc, db := newChannelModelTestService(t)
	svc.dataDir = t.TempDir()
	return svc, db
}

func TestSub2APIBaseURL(t *testing.T) {
	for _, raw := range []string{"https://relay.example", "https://relay.example/", "https://relay.example/v1/", " https://relay.example/v1 "} {
		got, err := normalizeSub2APIBaseURL(raw)
		if err != nil || got != "https://relay.example/v1" {
			t.Errorf("normalizeSub2APIBaseURL(%q) = %q, %v", raw, got, err)
		}
	}
	for _, raw := range []string{"", "/v1", "ftp://relay.example/v1", "https:///v1", "https://relay.example/api/v1", "https://relay.example/v2", "https://relay.example/v1/videos", "https://user:password@relay.example/v1", "https://relay.example/v1?key=secret", "https://relay.example/v1?", "https://relay.example/v1#fragment", "https://relay.example/v%31", "https://relay.example:invalid/v1", "https://relay.example:65536/v1", "https://relay.example:/v1"} {
		if _, err := normalizeSub2APIBaseURL(raw); err == nil {
			t.Errorf("normalizeSub2APIBaseURL(%q) should fail", raw)
		}
	}
}

func TestSub2APIBootstrapPersistsSyntheticLegacyPriceTier(t *testing.T) {
	svc, db := newSub2APIIntegrationTestService(t)
	if err := svc.EnsureSub2APIRelayChannel(); err != nil {
		t.Fatal(err)
	}
	item, err := svc.repo.ChannelModelByKey(sub2APIRelayChannelID, "gpt-5.5")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&item.PriceTiers[0]).Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := svc.EnsureSub2APIRelayChannel(); err != nil {
			t.Fatal(err)
		}
	}
	actual, err := svc.repo.ChannelModelByKey(sub2APIRelayChannelID, item.ModelKey)
	if err != nil || len(actual.PriceTiers) != 1 || actual.PriceTiers[0].ID == "" {
		t.Fatalf("price tier must have a persisted ID: %#v, err = %v", actual, err)
	}
}

func TestSub2APIBootstrapCapabilitiesAndSecrets(t *testing.T) {
	svc, _ := newSub2APIIntegrationTestService(t)
	t.Setenv("SUB2API_RELAY_MODELS", "models/gpt-test,gpt-test")
	t.Setenv("SUB2API_RELAY_IMAGE_MODELS", "gpt-image-1,grok-imagine-image")
	t.Setenv("SUB2API_RELAY_VIDEO_MODELS", "minimax_h3_b99_001,minimax_h3_b99_002,minimax_h3_b99_003_12s")
	for range 2 {
		if err := svc.EnsureSub2APIRelayChannel(); err != nil {
			t.Fatal(err)
		}
	}
	items, err := svc.repo.ChannelModels(sub2APIRelayChannelID, true)
	if err != nil || len(items) != 6 {
		t.Fatalf("channel models = %#v, err = %v", items, err)
	}
	for _, item := range items {
		wantCapability, wantProtocol := "video", model.ChannelInterfaceType("sub2api-video-v1")
		switch item.ModelKey {
		case "gpt-test":
			wantCapability, wantProtocol = "text", model.ChannelInterfaceChatCompletion
		case "gpt-image-1":
			wantCapability, wantProtocol = "image", model.ChannelInterfaceOpenAIImage
		case "grok-imagine-image":
			wantCapability, wantProtocol = "image", model.ChannelInterfaceGrokImage
		}
		if item.Capability != wantCapability || item.Protocol != wantProtocol || item.ProviderModelKey != item.ModelKey || !item.Enabled {
			t.Errorf("model contract = %#v", item)
		}
		if len(item.PriceTiers) != 1 || item.PriceTiers[0].UnitPriceMicrocredits != 0 || !item.PriceTiers[0].PriceConfigured {
			t.Errorf("model price tiers = %#v", item.PriceTiers)
		}
		config, err := DecodeModelCapabilityConfig(item.CapabilityConfigJSON)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := NormalizeModelCapabilityConfigForModel(item.Capability, string(item.Protocol), item.ModelKey, config); err != nil {
			t.Errorf("invalid capability for %s: %v", item.ModelKey, err)
		}
		if item.CapabilityVersion != 1 {
			t.Errorf("unchanged model capability version = %d, want 1", item.CapabilityVersion)
		}
	}
	stored, err := svc.repo.AdminSystemChannel(sub2APIRelayChannelID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.APIKey == "satellite-test-credential" || !strings.HasPrefix(stored.APIKey, encryptedSettingPrefix) {
		t.Fatal("system channel key was not encrypted at rest")
	}
	resolved, err := svc.SystemChannel(sub2APIRelayChannelID)
	if err != nil || resolved.APIKey != "satellite-test-credential" {
		t.Fatalf("server-side key cannot be recovered: %v", err)
	}
	public, err := svc.PublicSystemChannels()
	if err != nil || len(public) != 1 {
		t.Fatalf("public channels = %#v, err = %v", public, err)
	}
	if public[0].APIKey != "system" || public[0].BaseURL != "/api/ai/system/sub2api-relay" {
		t.Fatalf("unsafe public channel = %#v", public[0])
	}
	encoded, err := json.Marshal(public)
	if err != nil || strings.Contains(string(encoded), "sk-super-test-only") || strings.Contains(string(encoded), "sub2api:8080") {
		t.Fatalf("public channel leaked credentials or upstream location: %v", err)
	}
}

func TestSub2APIBootstrapPreservesOperatorPricing(t *testing.T) {
	svc, db := newSub2APIIntegrationTestService(t)
	if err := svc.EnsureSub2APIRelayChannel(); err != nil {
		t.Fatal(err)
	}
	item, err := svc.repo.ChannelModelByKey(sub2APIRelayChannelID, "gpt-5.5")
	if err != nil {
		t.Fatal(err)
	}
	item.BillingMode, item.UnitPriceMicrocredits, item.PriceVersion = "token", 99, 4
	item.InputTokenPriceMicrocredits, item.OutputTokenPriceMicrocredits = 13, 17
	item.PriceConfigured, item.DisplayName = false, "Operator display name"
	if err := svc.repo.SaveChannelModel(item); err != nil {
		t.Fatal(err)
	}
	tier := item.PriceTiers[0]
	tier.BillingMode, tier.UnitPriceMicrocredits, tier.PriceVersion = "token", 99, 4
	tier.InputTokenPriceMicrocredits, tier.OutputTokenPriceMicrocredits = 13, 17
	tier.Enabled, tier.PriceConfigured = false, false
	if err := db.Save(&tier).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.EnsureSub2APIRelayChannel(); err != nil {
		t.Fatal(err)
	}
	actual, err := svc.repo.ChannelModelByKey(sub2APIRelayChannelID, item.ModelKey)
	if err != nil {
		t.Fatal(err)
	}
	if actual.BillingMode != item.BillingMode || actual.UnitPriceMicrocredits != item.UnitPriceMicrocredits || actual.PriceVersion != item.PriceVersion || actual.PriceConfigured || actual.DisplayName != item.DisplayName || actual.InputTokenPriceMicrocredits != 13 || actual.OutputTokenPriceMicrocredits != 17 {
		t.Fatalf("operator model settings changed: %#v", actual)
	}
	if len(actual.PriceTiers) != 1 || actual.PriceTiers[0].ID != tier.ID || actual.PriceTiers[0].BillingMode != tier.BillingMode || actual.PriceTiers[0].PriceConfigured || actual.PriceTiers[0].Enabled || actual.PriceTiers[0].UnitPriceMicrocredits != 99 || actual.PriceTiers[0].InputTokenPriceMicrocredits != 13 || actual.PriceTiers[0].OutputTokenPriceMicrocredits != 17 {
		t.Fatalf("operator price tier changed: %#v", actual.PriceTiers)
	}
	// Repairing an absent tier must retain legacy operator prices, not make it free.
	if err := db.Delete(&tier).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.EnsureSub2APIRelayChannel(); err != nil {
		t.Fatal(err)
	}
	actual, err = svc.repo.ChannelModelByKey(sub2APIRelayChannelID, item.ModelKey)
	if err != nil || len(actual.PriceTiers) != 1 || actual.PriceTiers[0].BillingMode != "token" || actual.PriceTiers[0].UnitPriceMicrocredits != 99 || actual.PriceTiers[0].InputTokenPriceMicrocredits != 13 || actual.PriceTiers[0].OutputTokenPriceMicrocredits != 17 || actual.PriceTiers[0].PriceConfigured {
		t.Fatalf("repaired price tier = %#v, err = %v", actual, err)
	}
}

func TestSub2APIBootstrapDisableAndRestore(t *testing.T) {
	svc, _ := newSub2APIIntegrationTestService(t)
	t.Setenv("SUB2API_RELAY_IMAGE_MODELS", "gpt-image-1")
	if err := svc.EnsureSub2APIRelayChannel(); err != nil {
		t.Fatal(err)
	}
	original, err := svc.repo.ChannelModelByKey(sub2APIRelayChannelID, "gpt-image-1")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("SUB2API_RELAY_IMAGE_MODELS", "")
	if err := svc.EnsureSub2APIRelayChannel(); err != nil {
		t.Fatal(err)
	}
	disabled, err := svc.repo.ChannelModelByKeyIncludingDisabled(sub2APIRelayChannelID, original.ModelKey)
	if err != nil || disabled.Enabled {
		t.Fatalf("removed model was not disabled: %v", err)
	}
	t.Setenv("SUB2API_RELAY_BASE_URL", "")
	t.Setenv("SUB2API_APP_CREDENTIAL", "")
	if err := svc.EnsureSub2APIRelayChannel(); err != nil {
		t.Fatal(err)
	}
	channel, err := svc.repo.AdminSystemChannel(sub2APIRelayChannelID)
	if err != nil || channel.Enabled || channel.APIKey != "" || channel.BaseURL != "" {
		t.Fatalf("disabled channel retains secret or is enabled: %v", err)
	}
	t.Setenv("SUB2API_RELAY_BASE_URL", "http://sub2api:8080/v1")
	t.Setenv("SUB2API_APP_CREDENTIAL", "satellite-rotated-credential")
	t.Setenv("SUB2API_RELAY_IMAGE_MODELS", "gpt-image-1")
	if err := svc.EnsureSub2APIRelayChannel(); err != nil {
		t.Fatal(err)
	}
	restored, err := svc.repo.ChannelModelByKey(sub2APIRelayChannelID, original.ModelKey)
	if err != nil || restored.ID != original.ID || len(restored.PriceTiers) != 1 || restored.PriceTiers[0].ID != original.PriceTiers[0].ID {
		t.Fatalf("restoration duplicated model/tier: %#v, err = %v", restored, err)
	}
	channel, err = svc.SystemChannel(sub2APIRelayChannelID)
	if err != nil || channel.APIKey != "satellite-rotated-credential" {
		t.Fatalf("rotated key unavailable: %v", err)
	}
}

func TestSub2APIBootstrapRejectsInvalidConfigBeforeWriting(t *testing.T) {
	for _, test := range []struct{ name, env, value string }{
		{"gateway", "SUB2API_RELAY_BASE_URL", "http://sub2api:8080/api/v1"},
		{"missing app credential", "SUB2API_APP_CREDENTIAL", ""},
		{"ambiguous capability", "SUB2API_RELAY_IMAGE_MODELS", "models/gpt-5.5"},
		{"empty normalized name", "SUB2API_RELAY_MODELS", "models/"},
		{"name too long", "SUB2API_RELAY_MODELS", strings.Repeat("m", 121)},
		{"unsupported video", "SUB2API_RELAY_VIDEO_MODELS", "indextts2-v1"},
		{"video model casing", "SUB2API_RELAY_VIDEO_MODELS", "MINIMAX"},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc, db := newSub2APIIntegrationTestService(t)
			t.Setenv(test.env, test.value)
			if err := svc.EnsureSub2APIRelayChannel(); err == nil {
				t.Fatal("invalid configuration must fail")
			}
			var count int64
			if err := db.Model(&model.ModelChannel{}).Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("invalid configuration changed database: count = %d, err = %v", count, err)
			}
		})
	}
}

func TestSub2APIBootstrapZeroPriceAllowsEmptyWallet(t *testing.T) {
	svc, db := newSub2APIIntegrationTestService(t)
	if err := db.AutoMigrate(&model.SystemSetting{}, &model.CreditAccount{}, &model.BillingOrder{}, &model.CreditLedgerEntry{}); err != nil {
		t.Fatal(err)
	}
	if err := svc.EnsureSub2APIRelayChannel(); err != nil {
		t.Fatal(err)
	}
	order, err := svc.newBillingOrder("new-sso-user", "", "sub2api-zero-price-test", sub2APIRelayChannelID, "gpt-5.5", "text", "text_generation", 1, tokenBillingEstimate{})
	if err != nil {
		t.Fatal(err)
	}
	if order.AmountMicrocredits != 0 {
		t.Fatalf("bootstrap price = %d, want 0", order.AmountMicrocredits)
	}
	if err := svc.repo.ReserveBillingOrder(order); err != nil {
		t.Fatalf("zero-price request should allow a new empty wallet: %v", err)
	}
	account, err := svc.repo.CreditAccount(order.UserID)
	if err != nil || account.AvailableMicrocredits != 0 || account.ReservedMicrocredits != 0 {
		t.Fatalf("zero-price request changed wallet: %#v, err = %v", account, err)
	}
}

func TestSub2APIVideoCapabilityProfiles(t *testing.T) {
	for _, test := range []struct {
		name                             string
		minImages, maxImages, maxSeconds int
	}{
		{"minimax", 0, 0, 15}, {"minimax-h3", 0, 0, 15}, {"minimax_h3", 0, 0, 15},
		{"minimax_h3_b99_001", 0, 0, 15}, {"minimax_h3_b99_002", 2, 2, 15}, {"minimax_h3_b99_003_12s", 1, 9, 12},
	} {
		t.Run(test.name, func(t *testing.T) {
			config, err := sub2APIVideoCapabilityConfig(test.name)
			if err != nil {
				t.Fatal(err)
			}
			video := config.Video
			if video.References.MinImages != test.minImages || video.References.MaxImages != test.maxImages || video.Duration.Max != test.maxSeconds || video.Duration.Default != 5 || video.References.MaxVideos != 0 || video.References.MaxAudios != 0 {
				t.Fatalf("incorrect workflow capability: %#v", video)
			}
			if len(video.Resolutions) != 1 || video.DefaultResolution != "736p" || len(video.Ratios) != 2 || video.GenerateAudio.Supported || video.Watermark.Supported {
				t.Fatalf("unverified workflow parameters exposed: %#v", video)
			}
		})
	}
	if _, err := sub2APIVideoCapabilityConfig("unknown-video"); err == nil {
		t.Fatal("unverified model should not receive generic video capabilities")
	}
}
