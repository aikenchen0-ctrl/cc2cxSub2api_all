package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

const satelliteBillingConfigCacheTTL = 30 * time.Second

type cachedSatelliteBillingConfigs struct {
	expiresAt time.Time
	configs   map[string]SatelliteBillingConfig
}

type SatelliteBillingMode string
type SatelliteBillingUnit string

const (
	SatelliteBillingModeModel   SatelliteBillingMode = "model"
	SatelliteBillingModeCustom  SatelliteBillingMode = "custom"
	SatelliteBillingModeRequest SatelliteBillingMode = "request"
	SatelliteBillingModePoints  SatelliteBillingMode = "points"

	SatelliteBillingUnitMillionTokens SatelliteBillingUnit = "millionTokens"
	SatelliteBillingUnitImage         SatelliteBillingUnit = "image"
	SatelliteBillingUnitVideoSecond   SatelliteBillingUnit = "videoSecond"
	SatelliteBillingUnitRequest       SatelliteBillingUnit = "request"

	// Points are converted to the Sub2API USD balance at a fixed, visible rate.
	// 100 configured points therefore deduct exactly $1 from the user balance.
	SatelliteBillingUSDPerPoint = 0.01

	satelliteBillingSettingPrefix = "satellite_billing_v1_"
	satelliteBillingDBTimeout     = 5 * time.Second
)

var ErrInvalidSatelliteBillingConfig = errors.New("invalid satellite billing configuration")

var satelliteBillingSlugs = []string{
	"aicut", "aiexcel", "ai3d", "aihuoke", "canvas", "ju", "livart", "ppt", "qrcode", "screen2code", "yibiao",
}

type SatelliteBillingRule struct {
	Target string               `json:"target"`
	Unit   SatelliteBillingUnit `json:"unit"`
	Amount float64              `json:"amount"`
}

type SatelliteBillingConfig struct {
	Mode  SatelliteBillingMode   `json:"mode"`
	Rules []SatelliteBillingRule `json:"rules"`
}

type SatelliteBillingUsage struct {
	Model                string
	InboundEndpoint      string
	Tokens               UsageTokens
	ImageCount           int
	VideoCount           int
	VideoDurationSeconds int
}

func SatelliteBillingAppSlugs() []string {
	return append([]string(nil), satelliteBillingSlugs...)
}

func IsSatelliteBillingApp(slug string) bool {
	for _, supported := range satelliteBillingSlugs {
		if slug == supported {
			return true
		}
	}
	return false
}

func defaultSatelliteBillingConfig() SatelliteBillingConfig {
	return SatelliteBillingConfig{Mode: SatelliteBillingModeModel, Rules: []SatelliteBillingRule{}}
}

func satelliteBillingSettingKey(slug string) string {
	return satelliteBillingSettingPrefix + slug
}

func normalizeSatelliteBillingConfig(config SatelliteBillingConfig) (SatelliteBillingConfig, error) {
	switch config.Mode {
	case SatelliteBillingModeModel, SatelliteBillingModeCustom, SatelliteBillingModeRequest, SatelliteBillingModePoints:
	default:
		return SatelliteBillingConfig{}, fmt.Errorf("unknown satellite billing mode %q", config.Mode)
	}
	if len(config.Rules) > 100 {
		return SatelliteBillingConfig{}, errors.New("a satellite can have at most 100 pricing rules")
	}

	rules := make([]SatelliteBillingRule, 0, len(config.Rules))
	seen := make(map[string]struct{}, len(config.Rules))
	for index, rule := range config.Rules {
		rule.Target = strings.TrimSpace(rule.Target)
		if rule.Target == "" || len(rule.Target) > 160 {
			return SatelliteBillingConfig{}, fmt.Errorf("rule %d must have a target of 1 to 160 characters", index+1)
		}
		switch rule.Unit {
		case SatelliteBillingUnitMillionTokens, SatelliteBillingUnitImage, SatelliteBillingUnitVideoSecond, SatelliteBillingUnitRequest:
		default:
			return SatelliteBillingConfig{}, fmt.Errorf("rule %d has an unsupported billing unit", index+1)
		}
		if config.Mode != SatelliteBillingModeCustom && rule.Unit == SatelliteBillingUnitMillionTokens {
			return SatelliteBillingConfig{}, fmt.Errorf("rule %d uses a token unit outside custom pricing mode", index+1)
		}
		if math.IsNaN(rule.Amount) || math.IsInf(rule.Amount, 0) || rule.Amount < 0 || rule.Amount > 1_000_000_000 {
			return SatelliteBillingConfig{}, fmt.Errorf("rule %d amount must be between 0 and 1000000000", index+1)
		}
		if config.Mode == SatelliteBillingModePoints && math.Trunc(rule.Amount) != rule.Amount {
			return SatelliteBillingConfig{}, fmt.Errorf("rule %d points amount must be a whole number", index+1)
		}
		uniqueKey := strings.ToLower(rule.Target) + "\x00" + string(rule.Unit)
		if _, exists := seen[uniqueKey]; exists {
			return SatelliteBillingConfig{}, fmt.Errorf("rule %d duplicates a target and billing unit", index+1)
		}
		seen[uniqueKey] = struct{}{}
		rules = append(rules, rule)
	}

	config.Rules = rules
	return config, nil
}

func satelliteBillingContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(context.WithoutCancel(ctx), satelliteBillingDBTimeout)
}

func (s *SettingService) GetSatelliteBillingConfigs(ctx context.Context) (map[string]SatelliteBillingConfig, error) {
	if s == nil || s.settingRepo == nil {
		return nil, errors.New("satellite billing settings are unavailable")
	}
	if cached, ok := s.satelliteBillingCache.Load().(*cachedSatelliteBillingConfigs); ok && cached != nil && time.Now().Before(cached.expiresAt) {
		return cloneSatelliteBillingConfigs(cached.configs), nil
	}
	value, err, _ := s.satelliteBillingSF.Do("all", func() (any, error) {
		if cached, ok := s.satelliteBillingCache.Load().(*cachedSatelliteBillingConfigs); ok && cached != nil && time.Now().Before(cached.expiresAt) {
			return cached.configs, nil
		}
		configs, err := s.loadSatelliteBillingConfigs(ctx)
		if err != nil {
			return nil, err
		}
		s.satelliteBillingCache.Store(&cachedSatelliteBillingConfigs{
			expiresAt: time.Now().Add(satelliteBillingConfigCacheTTL),
			configs:   configs,
		})
		return configs, nil
	})
	if err != nil {
		return nil, err
	}
	configs, _ := value.(map[string]SatelliteBillingConfig)
	return cloneSatelliteBillingConfigs(configs), nil
}

func (s *SettingService) loadSatelliteBillingConfigs(ctx context.Context) (map[string]SatelliteBillingConfig, error) {
	dbCtx, cancel := satelliteBillingContext(ctx)
	defer cancel()

	keys := make([]string, 0, len(satelliteBillingSlugs))
	for _, slug := range satelliteBillingSlugs {
		keys = append(keys, satelliteBillingSettingKey(slug))
	}
	values, err := s.settingRepo.GetMultiple(dbCtx, keys)
	if err != nil {
		return nil, fmt.Errorf("load satellite billing settings: %w", err)
	}
	configs := make(map[string]SatelliteBillingConfig, len(satelliteBillingSlugs))
	for _, slug := range satelliteBillingSlugs {
		config := defaultSatelliteBillingConfig()
		if raw := values[satelliteBillingSettingKey(slug)]; raw != "" {
			if err := json.Unmarshal([]byte(raw), &config); err != nil {
				return nil, fmt.Errorf("decode satellite billing settings for %s: %w", slug, err)
			}
			config, err = normalizeSatelliteBillingConfig(config)
			if err != nil {
				return nil, fmt.Errorf("invalid stored satellite billing settings for %s: %w", slug, err)
			}
		}
		configs[slug] = config
	}
	return configs, nil
}

func cloneSatelliteBillingConfigs(configs map[string]SatelliteBillingConfig) map[string]SatelliteBillingConfig {
	cloned := make(map[string]SatelliteBillingConfig, len(configs))
	for slug, config := range configs {
		config.Rules = append([]SatelliteBillingRule(nil), config.Rules...)
		cloned[slug] = config
	}
	return cloned
}

func (s *SettingService) GetSatelliteBillingConfig(ctx context.Context, slug string) (SatelliteBillingConfig, error) {
	if !IsSatelliteBillingApp(slug) {
		return SatelliteBillingConfig{}, fmt.Errorf("unsupported satellite app %q", slug)
	}
	configs, err := s.GetSatelliteBillingConfigs(ctx)
	if err != nil {
		return SatelliteBillingConfig{}, err
	}
	config, ok := configs[slug]
	if !ok {
		return SatelliteBillingConfig{}, fmt.Errorf("satellite billing settings are missing for %s", slug)
	}
	return config, nil
}

func (s *SettingService) UpdateSatelliteBillingConfig(ctx context.Context, slug string, config SatelliteBillingConfig) (SatelliteBillingConfig, error) {
	if !IsSatelliteBillingApp(slug) {
		return SatelliteBillingConfig{}, fmt.Errorf("unsupported satellite app %q", slug)
	}
	if s == nil || s.settingRepo == nil {
		return SatelliteBillingConfig{}, errors.New("satellite billing settings are unavailable")
	}
	config, err := normalizeSatelliteBillingConfig(config)
	if err != nil {
		return SatelliteBillingConfig{}, fmt.Errorf("%w: %v", ErrInvalidSatelliteBillingConfig, err)
	}
	data, err := json.Marshal(config)
	if err != nil {
		return SatelliteBillingConfig{}, fmt.Errorf("encode satellite billing settings: %w", err)
	}
	dbCtx, cancel := satelliteBillingContext(ctx)
	defer cancel()
	if err := s.settingRepo.Set(dbCtx, satelliteBillingSettingKey(slug), string(data)); err != nil {
		return SatelliteBillingConfig{}, fmt.Errorf("save satellite billing settings for %s: %w", slug, err)
	}
	s.satelliteBillingCache.Store(&cachedSatelliteBillingConfigs{})
	return config, nil
}

func SatelliteBillingSlugFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	slug, _ := ctx.Value(ctxkey.SatelliteApp).(string)
	if IsSatelliteBillingApp(slug) {
		return slug
	}
	return ""
}

func applySatelliteBillingConfig(ctx context.Context, settings *SettingService, usage SatelliteBillingUsage) (*CostBreakdown, bool) {
	slug := SatelliteBillingSlugFromContext(ctx)
	if slug == "" || settings == nil {
		return nil, false
	}
	config, err := settings.GetSatelliteBillingConfig(ctx, slug)
	if err != nil {
		logger.LegacyPrintf("service.satellite_billing", "load billing config failed for %s; keeping model pricing: %v", slug, err)
		return nil, false
	}
	return quoteSatelliteBilling(config, usage)
}

func overrideCustomerCostWithSatelliteQuote(modelCost, satelliteQuote *CostBreakdown) *CostBreakdown {
	if satelliteQuote == nil {
		return modelCost
	}
	if modelCost == nil {
		modelCost = &CostBreakdown{}
	}
	modelCost.ActualCost = satelliteQuote.ActualCost
	modelCost.BillingMode = satelliteQuote.BillingMode
	return modelCost
}

func quoteSatelliteBilling(config SatelliteBillingConfig, usage SatelliteBillingUsage) (*CostBreakdown, bool) {
	if config.Mode == SatelliteBillingModeModel || len(config.Rules) == 0 {
		return nil, false
	}

	units := make([]SatelliteBillingUnit, 0, 3)
	switch {
	case usage.VideoCount > 0:
		units = append(units, SatelliteBillingUnitVideoSecond)
	case usage.ImageCount > 0:
		units = append(units, SatelliteBillingUnitImage)
	}
	if satelliteUsageTokenCount(usage.Tokens) > 0 {
		units = append(units, SatelliteBillingUnitMillionTokens)
	}
	units = append(units, SatelliteBillingUnitRequest)

	targets := []string{
		normalizeSatelliteBillingTarget(usage.InboundEndpoint),
		normalizeSatelliteBillingTarget(usage.Model),
		"*",
	}
	for _, target := range targets {
		if target == "" {
			continue
		}
		for _, unit := range units {
			for _, rule := range config.Rules {
				if normalizeSatelliteBillingTarget(rule.Target) != target || rule.Unit != unit {
					continue
				}
				return satelliteBillingCost(rule, config.Mode, usage), true
			}
		}
	}
	return nil, false
}

func normalizeSatelliteBillingTarget(target string) string {
	target = strings.ToLower(strings.TrimSpace(target))
	if query := strings.IndexByte(target, '?'); query >= 0 {
		target = target[:query]
	}
	return strings.TrimRight(target, "/")
}

func satelliteUsageTokenCount(tokens UsageTokens) int64 {
	// Image input/output and image-cache-read counts are details within the
	// aggregate input/output/cache buckets, so do not add them a second time.
	return int64(max(tokens.InputTokens, 0)) + int64(max(tokens.OutputTokens, 0)) +
		satelliteCacheCreationTokenCount(tokens) + int64(max(tokens.CacheReadTokens, 0))
}

func satelliteCacheCreationTokenCount(tokens UsageTokens) int64 {
	if tokens.CacheCreationTokens > 0 {
		return int64(tokens.CacheCreationTokens)
	}
	return int64(max(tokens.CacheCreation5mTokens, 0)) + int64(max(tokens.CacheCreation1hTokens, 0))
}

func satelliteBillingCostMode(unit SatelliteBillingUnit) string {
	switch unit {
	case SatelliteBillingUnitMillionTokens:
		return string(BillingModeToken)
	case SatelliteBillingUnitImage:
		return string(BillingModeImage)
	case SatelliteBillingUnitVideoSecond:
		return string(BillingModeVideo)
	default:
		return string(BillingModePerRequest)
	}
}

func satelliteBillingCost(rule SatelliteBillingRule, mode SatelliteBillingMode, usage SatelliteBillingUsage) *CostBreakdown {
	unitPrice := rule.Amount
	if mode == SatelliteBillingModePoints {
		unitPrice *= SatelliteBillingUSDPerPoint
	}
	cost := &CostBreakdown{BillingMode: satelliteBillingCostMode(rule.Unit)}
	switch rule.Unit {
	case SatelliteBillingUnitMillionTokens:
		scale := unitPrice / 1_000_000
		inputTokens := max(usage.Tokens.InputTokens, 0)
		imageInputTokens := min(max(usage.Tokens.ImageInputTokens, 0), inputTokens)
		outputTokens := max(usage.Tokens.OutputTokens, 0)
		imageOutputTokens := min(max(usage.Tokens.ImageOutputTokens, 0), outputTokens)
		textInputTokens := inputTokens - imageInputTokens
		textOutputTokens := outputTokens - imageOutputTokens
		cost.InputCost = float64(textInputTokens) * scale
		cost.ImageInputCost = float64(imageInputTokens) * scale
		cost.OutputCost = float64(textOutputTokens) * scale
		cost.CacheCreationCost = float64(satelliteCacheCreationTokenCount(usage.Tokens)) * scale
		cost.CacheReadCost = float64(max(usage.Tokens.CacheReadTokens, 0)) * scale
		cost.ImageOutputCost = float64(imageOutputTokens) * scale
		cost.TotalCost = cost.InputCost + cost.ImageInputCost + cost.OutputCost + cost.CacheCreationCost + cost.CacheReadCost + cost.ImageOutputCost
	case SatelliteBillingUnitImage:
		cost.TotalCost = unitPrice * float64(usage.ImageCount)
	case SatelliteBillingUnitVideoSecond:
		cost.TotalCost = unitPrice * float64(NormalizeVideoBillingDurationSecondsOrDefault(usage.VideoDurationSeconds)) * float64(usage.VideoCount)
	default:
		cost.TotalCost = unitPrice
	}
	cost.TotalCost = math.Round(cost.TotalCost*1_000_000) / 1_000_000
	cost.ActualCost = cost.TotalCost
	return cost
}
