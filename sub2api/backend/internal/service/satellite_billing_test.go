package service

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

type satelliteBillingTestSettings struct {
	mu     sync.Mutex
	values map[string]string
}

func (s *satelliteBillingTestSettings) Get(_ context.Context, key string) (*Setting, error) {
	value, err := s.GetValue(context.Background(), key)
	if err != nil {
		return nil, err
	}
	return &Setting{Key: key, Value: value}, nil
}

func (s *satelliteBillingTestSettings) GetValue(_ context.Context, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}

func (s *satelliteBillingTestSettings) Set(_ context.Context, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.values == nil {
		s.values = make(map[string]string)
	}
	s.values[key] = value
	return nil
}

func (s *satelliteBillingTestSettings) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	values := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			values[key] = value
		}
	}
	return values, nil
}

func (s *satelliteBillingTestSettings) SetMultiple(_ context.Context, values map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.values == nil {
		s.values = make(map[string]string)
	}
	for key, value := range values {
		s.values[key] = value
	}
	return nil
}

func (s *satelliteBillingTestSettings) GetAll(context.Context) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	values := make(map[string]string, len(s.values))
	for key, value := range s.values {
		values[key] = value
	}
	return values, nil
}

func (s *satelliteBillingTestSettings) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, key)
	return nil
}

func TestSatelliteBillingConfigValidation(t *testing.T) {
	valid := SatelliteBillingConfig{
		Mode:  SatelliteBillingModePoints,
		Rules: []SatelliteBillingRule{{Target: " * ", Unit: SatelliteBillingUnitRequest, Amount: 125}},
	}
	normalized, err := normalizeSatelliteBillingConfig(valid)
	if err != nil || normalized.Rules[0].Target != "*" {
		t.Fatalf("expected valid config to normalize target: config=%+v err=%v", normalized, err)
	}

	tests := []SatelliteBillingConfig{
		{Mode: SatelliteBillingModeCustom, Rules: []SatelliteBillingRule{{Target: "model", Unit: SatelliteBillingUnitRequest, Amount: -1}}},
		{Mode: SatelliteBillingModePoints, Rules: []SatelliteBillingRule{{Target: "model", Unit: SatelliteBillingUnitRequest, Amount: 1.5}}},
		{Mode: SatelliteBillingModeRequest, Rules: []SatelliteBillingRule{{Target: "model", Unit: SatelliteBillingUnitMillionTokens, Amount: 1}}},
		{Mode: SatelliteBillingModeCustom, Rules: []SatelliteBillingRule{
			{Target: "model", Unit: SatelliteBillingUnitRequest, Amount: 1},
			{Target: " MODEL ", Unit: SatelliteBillingUnitRequest, Amount: 2},
		}},
	}
	for index, config := range tests {
		if _, err := normalizeSatelliteBillingConfig(config); err == nil {
			t.Errorf("invalid config %d unexpectedly passed validation", index)
		}
	}
}

func TestSatelliteBillingConfigPersistsAndInvalidatesCache(t *testing.T) {
	repo := &satelliteBillingTestSettings{values: make(map[string]string)}
	settings := NewSettingService(repo, nil)

	initial, err := settings.GetSatelliteBillingConfig(context.Background(), "canvas")
	if err != nil || initial.Mode != SatelliteBillingModeModel {
		t.Fatalf("unexpected default config: config=%+v err=%v", initial, err)
	}

	want := SatelliteBillingConfig{
		Mode:  SatelliteBillingModeCustom,
		Rules: []SatelliteBillingRule{{Target: "gpt-image-1", Unit: SatelliteBillingUnitImage, Amount: 0.25}},
	}
	if _, err := settings.UpdateSatelliteBillingConfig(context.Background(), "canvas", want); err != nil {
		t.Fatalf("save config: %v", err)
	}
	got, err := settings.GetSatelliteBillingConfig(context.Background(), "canvas")
	if err != nil || len(got.Rules) != 1 || got.Rules[0] != want.Rules[0] {
		t.Fatalf("saved config was not returned from settings store: config=%+v err=%v", got, err)
	}

	var stored SatelliteBillingConfig
	if err := json.Unmarshal([]byte(repo.values[satelliteBillingSettingKey("canvas")]), &stored); err != nil || stored.Mode != want.Mode {
		t.Fatalf("config was not persisted as JSON: stored=%+v err=%v", stored, err)
	}
}

func TestQuoteSatelliteBillingMatchesPathThenModelThenWildcard(t *testing.T) {
	config := SatelliteBillingConfig{
		Mode: SatelliteBillingModeRequest,
		Rules: []SatelliteBillingRule{
			{Target: "*", Unit: SatelliteBillingUnitRequest, Amount: 1},
			{Target: "gpt-image-1", Unit: SatelliteBillingUnitRequest, Amount: 3},
			{Target: "/v1/images/generations", Unit: SatelliteBillingUnitRequest, Amount: 9},
		},
	}
	usage := SatelliteBillingUsage{
		Model: "gpt-image-1", InboundEndpoint: " /v1/images/generations?async=true ", ImageCount: 2,
	}
	quote, ok := quoteSatelliteBilling(config, usage)
	if !ok || quote.TotalCost != 9 || quote.ActualCost != 9 || quote.BillingMode != string(BillingModePerRequest) {
		t.Fatalf("expected API path rule to win: quote=%+v matched=%v", quote, ok)
	}

	usage.InboundEndpoint = "/v1/videos"
	quote, ok = quoteSatelliteBilling(config, usage)
	if !ok || quote.TotalCost != 3 {
		t.Fatalf("expected model rule to win over wildcard: quote=%+v matched=%v", quote, ok)
	}

	usage.Model = "unknown-model"
	quote, ok = quoteSatelliteBilling(config, usage)
	if !ok || quote.TotalCost != 1 {
		t.Fatalf("expected wildcard rule: quote=%+v matched=%v", quote, ok)
	}
}

func TestQuoteSatelliteBillingCalculatesTokensMediaAndPoints(t *testing.T) {
	tests := []struct {
		name   string
		config SatelliteBillingConfig
		usage  SatelliteBillingUsage
		want   float64
		mode   string
	}{
		{
			name: "million tokens",
			config: SatelliteBillingConfig{Mode: SatelliteBillingModeCustom, Rules: []SatelliteBillingRule{{
				Target: "*", Unit: SatelliteBillingUnitMillionTokens, Amount: 3,
			}}},
			usage: SatelliteBillingUsage{Tokens: UsageTokens{InputTokens: 1_000_000, OutputTokens: 1_000_000}},
			want:  6, mode: string(BillingModeToken),
		},
		{
			name: "image and cache token details are not double counted",
			config: SatelliteBillingConfig{Mode: SatelliteBillingModeCustom, Rules: []SatelliteBillingRule{{
				Target: "*", Unit: SatelliteBillingUnitMillionTokens, Amount: 1_000_000,
			}}},
			usage: SatelliteBillingUsage{Tokens: UsageTokens{
				InputTokens: 100, ImageInputTokens: 20, ImageCacheReadTokens: 3,
				OutputTokens: 10, ImageOutputTokens: 2, CacheCreationTokens: 5, CacheReadTokens: 10,
			}},
			want: 125, mode: string(BillingModeToken),
		},
		{
			name: "cache creation breakdown is used when its aggregate is absent",
			config: SatelliteBillingConfig{Mode: SatelliteBillingModeCustom, Rules: []SatelliteBillingRule{{
				Target: "*", Unit: SatelliteBillingUnitMillionTokens, Amount: 1_000_000,
			}}},
			usage: SatelliteBillingUsage{Tokens: UsageTokens{CacheCreation5mTokens: 4, CacheCreation1hTokens: 6}},
			want:  10, mode: string(BillingModeToken),
		},
		{
			name: "images",
			config: SatelliteBillingConfig{Mode: SatelliteBillingModeRequest, Rules: []SatelliteBillingRule{{
				Target: "*", Unit: SatelliteBillingUnitImage, Amount: 0.05,
			}}},
			usage: SatelliteBillingUsage{ImageCount: 3}, want: 0.15, mode: string(BillingModeImage),
		},
		{
			name: "video seconds",
			config: SatelliteBillingConfig{Mode: SatelliteBillingModeRequest, Rules: []SatelliteBillingRule{{
				Target: "*", Unit: SatelliteBillingUnitVideoSecond, Amount: 0.1,
			}}},
			usage: SatelliteBillingUsage{VideoCount: 1, VideoDurationSeconds: 6}, want: 0.6, mode: string(BillingModeVideo),
		},
		{
			name: "video count and default duration",
			config: SatelliteBillingConfig{Mode: SatelliteBillingModeRequest, Rules: []SatelliteBillingRule{{
				Target: "*", Unit: SatelliteBillingUnitVideoSecond, Amount: 0.1,
			}}},
			usage: SatelliteBillingUsage{VideoCount: 2}, want: 1.6, mode: string(BillingModeVideo),
		},
		{
			name: "points",
			config: SatelliteBillingConfig{Mode: SatelliteBillingModePoints, Rules: []SatelliteBillingRule{{
				Target: "*", Unit: SatelliteBillingUnitRequest, Amount: 125,
			}}},
			usage: SatelliteBillingUsage{Tokens: UsageTokens{InputTokens: 100}}, want: 1.25, mode: string(BillingModePerRequest),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			quote, ok := quoteSatelliteBilling(test.config, test.usage)
			if !ok || quote.TotalCost != test.want || quote.BillingMode != test.mode {
				t.Fatalf("unexpected quote: quote=%+v matched=%v want=%g mode=%s", quote, ok, test.want, test.mode)
			}
		})
	}

	if _, matched := quoteSatelliteBilling(SatelliteBillingConfig{
		Mode:  SatelliteBillingModeRequest,
		Rules: []SatelliteBillingRule{{Target: "some-other-model", Unit: SatelliteBillingUnitRequest, Amount: 1}},
	}, SatelliteBillingUsage{Model: "model"}); matched {
		t.Fatal("unmatched request should retain existing model pricing")
	}
}

func TestOverrideCustomerCostWithSatelliteQuoteKeepsModelCost(t *testing.T) {
	modelCost := &CostBreakdown{TotalCost: 4.5, ActualCost: 6.75, BillingMode: string(BillingModeToken)}
	quote := &CostBreakdown{TotalCost: 1.25, ActualCost: 1.25, BillingMode: string(BillingModePerRequest)}

	got := overrideCustomerCostWithSatelliteQuote(modelCost, quote)
	if got.TotalCost != 4.5 || got.ActualCost != 1.25 || got.BillingMode != string(BillingModePerRequest) {
		t.Fatalf("satellite quote must replace only the customer charge: %+v", got)
	}
	if got := overrideCustomerCostWithSatelliteQuote(nil, quote); got == nil || got.TotalCost != 0 || got.ActualCost != 1.25 {
		t.Fatalf("quote should create a customer cost when model pricing is unavailable: %+v", got)
	}
}

func TestSatelliteBillingSlugRequiresVerifiedSupportedContextValue(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxkey.SatelliteApp, "canvas")
	if got := SatelliteBillingSlugFromContext(ctx); got != "canvas" {
		t.Fatalf("expected supported verified app slug, got %q", got)
	}
	for _, slug := range []string{"", "agentapi", "unknown"} {
		ctx := context.WithValue(context.Background(), ctxkey.SatelliteApp, slug)
		if got := SatelliteBillingSlugFromContext(ctx); got != "" {
			t.Errorf("unsupported app slug %q should not receive satellite pricing, got %q", slug, got)
		}
	}
	if _, err := NewSettingService(&satelliteBillingTestSettings{}, nil).GetSatelliteBillingConfig(context.Background(), "agentapi"); err == nil {
		t.Fatal("unsupported app config should be rejected")
	}
}
