package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
)

// PlazaOfficialPricing 模型广场展示用的官方参考价（USD per token），与计费同源：
// LiteLLM → 内置兜底价卡 → 模型策略。字段为 nil 表示该项缺失（0 视为未配置）。
type PlazaOfficialPricing struct {
	InputPrice        *float64
	OutputPrice       *float64
	CacheWritePrice   *float64 // 5m 缓存写入（= LiteLLM cache_creation）
	CacheWrite1hPrice *float64 // 1h 缓存写入，仅计费会区分 5m/1h 时给出
	CacheReadPrice    *float64
	// Intervals 官方长上下文阶梯（多档时给出），不受分组开关影响。
	Intervals []PricingInterval
}

// PlazaModel 模型广场中单个模型条目：按实收口径合成的展示定价 + 官方参考价。
type PlazaModel struct {
	Name            string
	Platform        string
	Manufacturer    string
	Capability      string
	Pricing         *ChannelModelPricing
	OfficialPricing *PlazaOfficialPricing
	// LongContextBasis 多档时的计价基准（整单 / 仅超出部分），单档为空。
	LongContextBasis ContextPricingBasis
	// TimePricing 计费会生效的分时倍率时段；无分时为 nil。
	TimePricing *TimePricingSchedule
}

// PlazaGroup 模型广场中以分组为顶层的条目。
//
// 与 AvailableGroupRef 相比多了 Description 与 Models；Models 来自该分组关联渠道的
// 支持模型、分组级价格卡、以及分组绑定账号的模型映射，因此无论部署使用
// 「渠道绑定分组」还是「账号直挂分组」，广场都能列出中转站实际支持的模型。
type PlazaGroup struct {
	ID                 int64
	Name               string
	Description        string
	Platform           string
	SubscriptionType   string
	RateMultiplier     float64
	PeakRateEnabled    bool
	PeakStart          string
	PeakEnd            string
	PeakRateMultiplier float64
	IsExclusive        bool
	// 图片按次实付倍率：ImageRateIndependent 为 true 时，图片计费模型的实付
	// = 档位价 × ImageRateMultiplier，不乘分组/用户专属倍率（与计费口径一致）。
	ImageRateIndependent bool
	ImageRateMultiplier  float64
	// 视频独立倍率与图片独立倍率分别配置，开启时覆盖分组/用户专属倍率。
	VideoRateIndependent bool
	VideoRateMultiplier  float64
	// LongContextPricingEnabled 分组是否按上下文长度应用阶梯价；关闭时模型展示的是最低档。
	LongContextPricingEnabled bool
	Models                    []PlazaModel
}

// ModelPlazaService 聚合模型广场数据。
//
// 模型枚举来自三处：渠道配置、分组级价格卡、分组绑定账号的模型映射；
// token 模型的展示单价与阶梯由 BillingService 的阶梯表查询给出（与扣费走同一条
// 解析链与计费函数），图片/按次模型沿用渠道/分组档位价。
type ModelPlazaService struct {
	channelRepo    ChannelRepository
	groupRepo      GroupRepository
	pricingService *PricingService
	billingService *BillingService
	resolver       *ModelPricingResolver
	accountRepo    AccountRepository
}

// NewModelPlazaService 创建模型广场服务。
//
// accountRepo 为可选参数：传入时会把「账号直挂分组」的模型映射也纳入枚举，
// 覆盖从未配置 channel_model_pricing 的历史部署。
func NewModelPlazaService(
	channelRepo ChannelRepository,
	groupRepo GroupRepository,
	pricingService *PricingService,
	billingService *BillingService,
	resolver *ModelPricingResolver,
	accountRepos ...AccountRepository,
) *ModelPlazaService {
	var accountRepo AccountRepository
	if len(accountRepos) > 0 {
		accountRepo = accountRepos[0]
	}
	return &ModelPlazaService{
		channelRepo:    channelRepo,
		groupRepo:      groupRepo,
		pricingService: pricingService,
		billingService: billingService,
		resolver:       resolver,
		accountRepo:    accountRepo,
	}
}

// plazaConcretePlatforms 是组合分组展开账号模型时遍历的具体平台集合，
// 与 isConcreteRequestPlatform 的判定保持一致。
var plazaConcretePlatforms = []string{
	"anthropic", "openai", "gemini", "antigravity", "grok",
	"kimi", "zhipu", "deepseek", "minimax", "opencode_go",
}

// ListGroups 返回模型广场数据：每个活跃分组附带其可用模型与定价。
//
// 模型枚举口径与 ListAvailable 一致（Active 渠道、SupportedModels ∪ 全局定价回落、
// 平台隔离），并把顶层从渠道换成分组：
//   - 渠道按 lower(name) 排序后遍历，保证同名模型去重结果确定；
//   - 同分组同名模型「先见者胜」，仅当已存条目无定价而新条目有定价时升级替换；
//   - 分组级价格卡（groups.model_pricing）也是模型声明，覆盖从未配置渠道定价的部署；
//   - 分组绑定账号的模型映射（含平台默认模型）同样纳入，覆盖账号直挂分组的历史部署；
//   - token 模型的单价与阶梯按实收口径合成（见 ResolveContextPricingSchedule），
//     图片计费模型的档位价按实收口径合成（见 plazaImageDisplayPricing）；
//   - 每个模型附带厂家、功能归类与官方参考价（查不到为 nil）；
//   - 分组按 RateMultiplier 升序（同倍率按名称），组内模型按名称排序。
//
// 可见性过滤（专属分组）不在此层做，由 handler 按登录态裁剪。
func (s *ModelPlazaService) ListGroups(ctx context.Context) ([]PlazaGroup, error) {
	channels, err := s.channelRepo.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}
	groups, err := s.groupRepo.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active groups: %w", err)
	}

	sort.SliceStable(channels, func(i, j int) bool {
		return strings.ToLower(channels[i].Name) < strings.ToLower(channels[j].Name)
	})

	byGroup := make(map[int64]*PlazaGroup, len(groups))
	groupEnt := make(map[int64]*Group, len(groups))
	order := make([]int64, 0, len(groups))
	for i := range groups {
		g := &groups[i]
		byGroup[g.ID] = &PlazaGroup{
			ID:                        g.ID,
			Name:                      g.Name,
			Description:               g.Description,
			Platform:                  g.Platform,
			SubscriptionType:          g.SubscriptionType,
			RateMultiplier:            g.RateMultiplier,
			PeakRateEnabled:           g.PeakRateEnabled,
			PeakStart:                 g.PeakStart,
			PeakEnd:                   g.PeakEnd,
			PeakRateMultiplier:        g.PeakRateMultiplier,
			IsExclusive:               g.IsExclusive,
			ImageRateIndependent:      g.ImageRateIndependent,
			ImageRateMultiplier:       g.ImageRateMultiplier,
			VideoRateIndependent:      g.VideoRateIndependent,
			VideoRateMultiplier:       g.VideoRateMultiplier,
			LongContextPricingEnabled: g.LongContextPricingEnabled,
		}
		groupEnt[g.ID] = g
		order = append(order, g.ID)
	}

	type modelKey struct {
		platform string
		name     string
	}
	// modelIdx[groupID][platform+modelName] = index into byGroup[groupID].Models
	modelIdx := make(map[int64]map[modelKey]int, len(groups))
	addModel := func(pg *PlazaGroup, gid int64, m SupportedModel) {
		if pg == nil || strings.TrimSpace(m.Name) == "" {
			return
		}
		if pg.Platform == PlatformComposite {
			if !isConcreteRequestPlatform(m.Platform) {
				return
			}
		} else if m.Platform != pg.Platform {
			return
		}
		idx := modelIdx[gid]
		if idx == nil {
			idx = make(map[modelKey]int)
			modelIdx[gid] = idx
		}
		key := modelKey{platform: m.Platform, name: strings.ToLower(m.Name)}
		if at, seen := idx[key]; seen {
			// 先见者胜；仅当已存条目无定价而新条目有定价时升级。
			if pg.Models[at].Pricing == nil && m.Pricing != nil {
				pg.Models[at].Pricing = m.Pricing
			}
			return
		}
		idx[key] = len(pg.Models)
		pg.Models = append(pg.Models, PlazaModel{Name: m.Name, Platform: m.Platform, Pricing: m.Pricing})
	}

	for i := range channels {
		ch := &channels[i]
		if ch.Status != StatusActive {
			continue
		}
		ch.normalizeBillingModelSource()
		supported := ch.SupportedModels()
		fillGlobalPricingFallback(s.pricingService, supported)

		for _, gid := range ch.GroupIDs {
			pg, ok := byGroup[gid]
			if !ok {
				continue
			}
			for j := range supported {
				addModel(pg, gid, supported[j])
			}
		}
	}

	// 分组级价格卡同样是模型声明，覆盖从未写入 channel_model_pricing 的部署。
	for gid, pg := range byGroup {
		g := groupEnt[gid]
		for i := range g.ModelPricing {
			p := g.ModelPricing[i]
			platform := p.Platform
			if platform == "" {
				platform = g.Platform
			}
			for _, name := range p.Models {
				if strings.Contains(name, "*") {
					continue
				}
				cp := p.Clone()
				cp.Models = []string{name}
				addModel(pg, gid, SupportedModel{Name: name, Platform: platform, Pricing: &cp})
			}
		}
	}

	// 历史部署把账号直接绑定到分组。把账号的模型映射（无映射时用平台默认模型）
	// 也纳入枚举，广场才能反映真实的调度面。
	if s.accountRepo != nil {
		for gid, pg := range byGroup {
			g := groupEnt[gid]
			platforms := []string{g.Platform}
			if g.Platform == PlatformComposite {
				platforms = plazaConcretePlatforms
			}
			accounts, err := s.accountRepo.ListModelAvailabilityCandidates(ctx, &gid, platforms, false)
			if err != nil {
				continue
			}
			for i := range accounts {
				mapping := accounts[i].GetModelMapping()
				if len(mapping) == 0 {
					mapping = defaultPlazaModelMapping(accounts[i].Platform)
				}
				platform := accounts[i].Platform
				if g.Platform != PlatformComposite {
					platform = g.Platform
				}
				for name := range mapping {
					if strings.Contains(name, "*") {
						continue
					}
					addModel(pg, gid, SupportedModel{Name: name, Platform: platform, Pricing: plazaMediaPricing(name, g)})
				}
			}
		}
	}

	officialMemo := make(map[string]*PlazaOfficialPricing)
	out := make([]PlazaGroup, 0, len(order))
	for _, gid := range order {
		pg := byGroup[gid]
		// 没有任何可枚举模型的分组不出现在广场：空价目卡对用户没有信息量。
		if len(pg.Models) == 0 {
			continue
		}
		sort.SliceStable(pg.Models, func(i, j int) bool {
			if pg.Models[i].Name != pg.Models[j].Name {
				return pg.Models[i].Name < pg.Models[j].Name
			}
			return pg.Models[i].Platform < pg.Models[j].Platform
		})
		g := groupEnt[gid]
		for j := range pg.Models {
			s.fillDisplayPricing(ctx, &pg.Models[j], g)
			pg.Models[j].Manufacturer = plazaManufacturer(pg.Models[j].Name, pg.Models[j].Platform)
			pg.Models[j].Capability = plazaCapability(pg.Models[j])
			pg.Models[j].OfficialPricing = s.lookupOfficialPricing(ctx, pg.Models[j].Name, officialMemo)
		}
		out = append(out, *pg)
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].RateMultiplier != out[j].RateMultiplier {
			return out[i].RateMultiplier < out[j].RateMultiplier
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// plazaManufacturer 由模型名推断厂家，未命中时回退到分组平台的展示名。
func plazaManufacturer(model, platform string) string {
	name := strings.ToLower(model)
	for _, provider := range []struct {
		label   string
		needles []string
	}{
		{"[OI]", []string{"gpt-", "chatgpt", "dall-e", "whisper", "text-embedding-"}},
		{"Gemini", []string{"gemini", "imagen", "veo"}},
		{"DeepSeek", []string{"deepseek"}},
		{"Anthropic", []string{"claude"}},
		{"Grok", []string{"grok"}},
		{"Kimi", []string{"kimi", "moonshot"}},
		{"智谱 GLM", []string{"glm-", "cogview", "cogvideo"}},
		{"MiniMax", []string{"minimax", "abab", "hailuo"}},
		{"阿里 Qwen", []string{"qwen", "wanx", "wan-", "qwen"}},
		{"字节豆包", []string{"doubao", "seedance", "seedream"}},
		{"Meta", []string{"llama"}},
		{"Cohere", []string{"command-r", "command-a"}},
	} {
		for _, needle := range provider.needles {
			if strings.Contains(name, needle) {
				return provider.label
			}
		}
	}
	return plazaPlatformDisplayName(platform)
}

// plazaPlatformDisplayName 平台展示名，仅用于厂家推断的兜底分支。
func plazaPlatformDisplayName(platform string) string {
	switch platform {
	case "anthropic":
		return "Anthropic"
	case "openai":
		return "[OI]"
	case "gemini":
		return "Gemini"
	case "antigravity":
		return "Antigravity"
	case "grok":
		return "Grok"
	case "kimi":
		return "Kimi"
	case "zhipu":
		return "智谱 GLM"
	case "deepseek":
		return "DeepSeek"
	case "minimax":
		return "MiniMax"
	case "opencode_go":
		return "OpenCode"
	}
	return platform
}

// plazaCapability 由模型名与计费模式推断文本/图片/视频归类，供广场的功能筛选使用。
func plazaCapability(m PlazaModel) string {
	name := strings.ToLower(m.Name)
	if strings.Contains(name, "video") || strings.HasPrefix(name, "sora") || strings.HasPrefix(name, "veo") ||
		strings.Contains(name, "seedance") || strings.Contains(name, "cogvideo") || strings.Contains(name, "hailuo") {
		return "video"
	}
	if strings.Contains(name, "image") || strings.Contains(name, "imagen") || strings.Contains(name, "dall-e") ||
		strings.Contains(name, "cogview") || strings.Contains(name, "seedream") {
		return "image"
	}
	switch m.PricingAsBillingMode() {
	case BillingModeImage:
		return "image"
	case BillingModeVideo:
		return "video"
	default:
		return "text"
	}
}

// PricingAsBillingMode 返回模型生效的计费模式；未配置时按 token 计费。
func (m PlazaModel) PricingAsBillingMode() BillingMode {
	if m.Pricing == nil || m.Pricing.BillingMode == "" {
		return BillingModeToken
	}
	return m.Pricing.BillingMode
}

// defaultPlazaModelMapping 账号未配置模型映射时使用的平台默认模型。
func defaultPlazaModelMapping(platform string) map[string]string {
	models := make(map[string]string)
	switch platform {
	case "openai":
		for _, name := range openai.DefaultModelIDs() {
			models[name] = name
		}
	case "grok":
		for name, target := range xai.DefaultModelMapping() {
			models[name] = target
		}
	}
	return models
}

// plazaMediaPricing 为账号枚举出的图片/视频模型合成分组档位价。
func plazaMediaPricing(model string, g *Group) *ChannelModelPricing {
	if g == nil {
		return nil
	}
	lower := strings.ToLower(model)
	if strings.Contains(lower, "video") {
		p := &ChannelModelPricing{BillingMode: BillingModeVideo}
		for _, tier := range []struct {
			label string
			value *float64
		}{
			{"480p", g.VideoPrice480P}, {"720p", g.VideoPrice720P}, {"1080p", g.VideoPrice1080P},
		} {
			if tier.value != nil {
				v := *tier.value
				p.Intervals = append(p.Intervals, PricingInterval{TierLabel: tier.label, PerRequestPrice: &v})
			}
		}
		return p
	}
	if strings.Contains(lower, "image") || strings.Contains(lower, "imagine") {
		p := &ChannelModelPricing{BillingMode: BillingModeImage}
		for _, tier := range []struct {
			label string
			value *float64
		}{
			{"1K", g.ImagePrice1K}, {"2K", g.ImagePrice2K}, {"4K", g.ImagePrice4K},
		} {
			if tier.value != nil {
				v := *tier.value
				p.Intervals = append(p.Intervals, PricingInterval{TierLabel: tier.label, PerRequestPrice: &v})
			}
		}
		return p
	}
	return nil
}

// fillDisplayPricing 把模型的展示定价换成实收口径：
// token 模型取计费阶梯表（单价与档位均由真实计费函数得出），
// 图片/按次模型（或阶梯表不可用时）沿用渠道定价与分组图片档位价。
func (s *ModelPlazaService) fillDisplayPricing(ctx context.Context, m *PlazaModel, g *Group) {
	if groupPricing := matchGroupModelPricing(g, m.Name); groupPricing != nil {
		m.Pricing = groupPricing
	}
	if s.billingService != nil && s.resolver != nil {
		sched, err := s.billingService.ResolveContextPricingSchedule(ctx, s.resolver, ContextPricingScheduleInput{
			Model:    m.Name,
			Group:    g,
			Platform: m.Platform,
		})
		if err == nil && sched != nil && len(sched.Tiers) > 0 {
			m.Pricing = withDefaultMaxReasoningEffortMultiplier(plazaPricingFromSchedule(m.Pricing, sched), m.Name)
			if len(sched.Tiers) > 1 {
				m.LongContextBasis = sched.Basis
			}
			m.TimePricing = sched.TimePricing
			return
		}
	}
	m.Pricing = withDefaultMaxReasoningEffortMultiplier(plazaImageDisplayPricing(m.Pricing, g), m.Name)
}

func withDefaultMaxReasoningEffortMultiplier(pricing *ChannelModelPricing, model string) *ChannelModelPricing {
	if pricing == nil || pricing.MaxReasoningEffortMultiplier != nil {
		return pricing
	}
	multiplier := defaultMaxReasoningEffortMultiplier(model)
	if multiplier == nil {
		return pricing
	}
	cloned := pricing.Clone()
	cloned.MaxReasoningEffortMultiplier = multiplier
	return &cloned
}

// plazaPricingFromSchedule 把阶梯表压成展示用的 ChannelModelPricing：
// 平价取首档单价，多档时 Intervals 逐档给出绝对单价；图片/按次字段沿用原始定价。
func plazaPricingFromSchedule(raw *ChannelModelPricing, sched *ContextPricingSchedule) *ChannelModelPricing {
	out := &ChannelModelPricing{BillingMode: BillingModeToken}
	if raw != nil {
		out.ImageInputPrice = raw.ImageInputPrice
		out.ImageOutputPrice = raw.ImageOutputPrice
		out.PerRequestPrice = raw.PerRequestPrice
		out.MaxReasoningEffortMultiplier = raw.MaxReasoningEffortMultiplier
	}
	first := sched.Tiers[0]
	out.InputPrice = first.Input
	out.OutputPrice = first.Output
	out.CacheWritePrice = first.CacheWrite
	out.CacheWrite1hPrice = first.CacheWrite1h
	out.CacheReadPrice = first.CacheRead
	if len(sched.Tiers) > 1 {
		out.Intervals = plazaIntervalsFromTiers(sched.Tiers)
	}
	return out
}

func plazaIntervalsFromTiers(tiers []ContextPricingTier) []PricingInterval {
	intervals := make([]PricingInterval, 0, len(tiers))
	for i, t := range tiers {
		intervals = append(intervals, PricingInterval{
			MinTokens:         t.MinTokens,
			MaxTokens:         t.MaxTokens,
			TierLabel:         t.Label,
			InputPrice:        t.Input,
			OutputPrice:       t.Output,
			CacheWritePrice:   t.CacheWrite,
			CacheWrite1hPrice: t.CacheWrite1h,
			CacheReadPrice:    t.CacheRead,
			SortOrder:         i,
		})
	}
	return intervals
}

// plazaImageDisplayPricing 为图片计费模型合成展示定价，使档位价与实收口径一致：
// 每档（1K/2K/4K）单价 = 分组图片价 > 渠道同档位价 > 渠道默认按次价，无价的档不展示。
// 分组未配任何图片价、或定价非图片模式时原样返回。返回克隆，不修改入参
// （渠道定价指针指向缓存共享数据）。
func plazaImageDisplayPricing(p *ChannelModelPricing, g *Group) *ChannelModelPricing {
	if p == nil || g == nil || p.BillingMode != BillingModeImage {
		return p
	}
	if g.ImagePrice1K == nil && g.ImagePrice2K == nil && g.ImagePrice4K == nil {
		return p
	}
	channelTierPrice := func(label string) *float64 {
		for i := range p.Intervals {
			if p.Intervals[i].TierLabel == label && p.Intervals[i].PerRequestPrice != nil {
				return p.Intervals[i].PerRequestPrice
			}
		}
		return p.PerRequestPrice
	}
	tiers := []struct {
		label      string
		groupPrice *float64
	}{
		{"1K", g.ImagePrice1K},
		{"2K", g.ImagePrice2K},
		{"4K", g.ImagePrice4K},
	}
	clone := *p
	clone.Intervals = make([]PricingInterval, 0, len(tiers))
	for i, t := range tiers {
		price := t.groupPrice
		if price == nil {
			price = channelTierPrice(t.label)
		}
		if price == nil {
			continue
		}
		v := *price
		clone.Intervals = append(clone.Intervals, PricingInterval{
			TierLabel:       t.label,
			PerRequestPrice: &v,
			SortOrder:       i,
		})
	}
	return &clone
}

// lookupOfficialPricing 查询模型的官方参考价（与计费同源：LiteLLM → 内置兜底 → 模型策略），
// 带 memo 避免同名模型重复解析。官方阶梯按无分组、无渠道的口径查阶梯表。
// billingService 为 nil（测试场景）或查不到时返回 nil。
func (s *ModelPlazaService) lookupOfficialPricing(ctx context.Context, modelName string, memo map[string]*PlazaOfficialPricing) *PlazaOfficialPricing {
	if s.billingService == nil {
		return nil
	}
	if cached, ok := memo[modelName]; ok {
		return cached
	}
	var result *PlazaOfficialPricing
	if mp, err := s.billingService.GetModelPricing(modelName); err == nil && mp != nil {
		result = &PlazaOfficialPricing{
			InputPrice:      nonZeroPtr(mp.InputPricePerToken),
			OutputPrice:     nonZeroPtr(mp.OutputPricePerToken),
			CacheWritePrice: nonZeroPtr(mp.CacheCreationPricePerToken),
			CacheReadPrice:  nonZeroPtr(mp.CacheReadPricePerToken),
		}
		// 计费只在支持 5m/1h 分档时使用 1h 价，其余情况 1h 价对用户无意义。
		if mp.SupportsCacheBreakdown {
			result.CacheWrite1hPrice = nonZeroPtr(mp.CacheCreation1hPrice)
		}
		if s.resolver != nil {
			sched, schedErr := s.billingService.ResolveContextPricingSchedule(ctx, s.resolver, ContextPricingScheduleInput{Model: modelName})
			if schedErr == nil && sched != nil && len(sched.Tiers) > 1 {
				result.Intervals = plazaIntervalsFromTiers(sched.Tiers)
			}
		}
		if result.InputPrice == nil && result.OutputPrice == nil && result.CacheWritePrice == nil &&
			result.CacheWrite1hPrice == nil && result.CacheReadPrice == nil && len(result.Intervals) == 0 {
			result = nil
		}
	}
	memo[modelName] = result
	return result
}
