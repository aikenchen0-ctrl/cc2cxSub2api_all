package service

import (
	"strings"
	"sync"
	"time"
)

const (
	// openAIModelTransientStreakTTL bounds how long a failure streak survives
	// without a new failure. It exists only so the map does not keep state for
	// account+model pairs that stopped being used; a streak is otherwise reset
	// by recordSuccess alone.
	//
	// It must stay well above the cooldowns. Resetting the streak on a short
	// wall-clock window makes the breaker's sensitivity depend on request rate:
	// a gateway called less often than the window never reaches streak 2, so a
	// broken upstream is never cooled down and every request pays a failed
	// attempt plus a failover before reaching a healthy account. Low-traffic
	// deployments were hit hardest, which is the opposite of what a breaker
	// should do.
	openAIModelTransientStreakTTL     = 30 * time.Minute
	openAIModelTransientShortCooldown = 10 * time.Second
	openAIModelTransientLongCooldown  = 45 * time.Second
	openAIModelTransientDefaultMax    = 4096
	openAIModelTransientMaxModelBytes = 512
	openAIModelCooldownPersistBackoff = time.Second
)

type openAIAccountModelKey struct {
	AccountID int64
	Model     string
}

type openAIAccountModelTransientEntry struct {
	failureStreak           int
	lastFailure             time.Time
	blockUntil              time.Time
	cooldownFloor           time.Time
	lastTouched             time.Time
	cooldownPersistedUntil  time.Time
	cooldownPersistingUntil time.Time
	cooldownPersistRetryAt  time.Time
}

type openAIAccountModelTransientDecision struct {
	FailureStreak int
	Cooldown      time.Duration
	BlockUntil    time.Time
}

type openAIAccountModelTransientState struct {
	mu         sync.Mutex
	entries    map[openAIAccountModelKey]openAIAccountModelTransientEntry
	maxEntries int
}

func newOpenAIAccountModelTransientState(maxEntries int) *openAIAccountModelTransientState {
	if maxEntries <= 0 {
		maxEntries = openAIModelTransientDefaultMax
	}
	return &openAIAccountModelTransientState{
		entries:    make(map[openAIAccountModelKey]openAIAccountModelTransientEntry),
		maxEntries: maxEntries,
	}
}

func normalizeOpenAIAccountModelTransientModel(model string) string {
	model = strings.TrimSpace(model)
	if len(model) > openAIModelTransientMaxModelBytes {
		return ""
	}
	return strings.ToLower(model)
}

func openAIAccountModelTransientKey(accountID int64, model string) (openAIAccountModelKey, bool) {
	model = normalizeOpenAIAccountModelTransientModel(model)
	if accountID <= 0 || model == "" {
		return openAIAccountModelKey{}, false
	}
	return openAIAccountModelKey{AccountID: accountID, Model: model}, true
}

func (s *openAIAccountModelTransientState) recordFailure(accountID int64, model string, now time.Time) openAIAccountModelTransientDecision {
	key, ok := openAIAccountModelTransientKey(accountID, model)
	if s == nil || !ok {
		return openAIAccountModelTransientDecision{}
	}
	if now.IsZero() {
		now = time.Now()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.entries == nil {
		s.entries = make(map[openAIAccountModelKey]openAIAccountModelTransientEntry)
	}
	if s.maxEntries <= 0 {
		s.maxEntries = openAIModelTransientDefaultMax
	}

	entry, exists := s.entries[key]
	if !exists {
		s.evictOldestLocked()
	}
	// The streak is cleared by recordSuccess. Only drop it here when the entry
	// is stale beyond the TTL, or when the clock moved backwards.
	if !exists || entry.lastFailure.IsZero() || now.Sub(entry.lastFailure) > openAIModelTransientStreakTTL || now.Before(entry.lastFailure) {
		entry.failureStreak = 0
		entry.blockUntil = time.Time{}
	}
	entry.failureStreak++
	entry.lastFailure = now
	entry.lastTouched = now

	cooldown := time.Duration(0)
	switch {
	case entry.failureStreak >= 3:
		cooldown = openAIModelTransientLongCooldown
	case entry.failureStreak == 2:
		cooldown = openAIModelTransientShortCooldown
	}
	if cooldown > 0 {
		entry.blockUntil = now.Add(cooldown)
	} else {
		entry.blockUntil = time.Time{}
	}
	if entry.cooldownFloor.After(entry.blockUntil) {
		entry.blockUntil = entry.cooldownFloor
	}
	s.entries[key] = entry
	return openAIAccountModelTransientDecision{
		FailureStreak: entry.failureStreak,
		Cooldown:      cooldown,
		BlockUntil:    entry.blockUntil,
	}
}

// recordCooldown 复用现有模型状态保存显式限流期限，不影响同账号其他模型。
// 后到的较短期限和早先在途请求的成功均不得提前解除 Retry-After。
func (s *openAIAccountModelTransientState) recordCooldown(accountID int64, model string, now, until time.Time) {
	key, ok := openAIAccountModelTransientKey(accountID, model)
	if s == nil || !ok || !until.After(now) {
		return
	}
	// 向上归一到秒，避免毫秒级到达差异触发重复写入，也不缩短上游期限。
	if rounded := until.Truncate(time.Second); rounded.Before(until) {
		until = rounded.Add(time.Second)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.entries == nil {
		s.entries = make(map[openAIAccountModelKey]openAIAccountModelTransientEntry)
	}
	if s.maxEntries <= 0 {
		s.maxEntries = openAIModelTransientDefaultMax
	}
	entry, exists := s.entries[key]
	if !exists {
		s.evictOldestLocked()
	}
	entry.lastFailure, entry.lastTouched = now, now
	if until.After(entry.cooldownFloor) {
		entry.cooldownFloor = until
	}
	if entry.cooldownFloor.After(entry.blockUntil) {
		entry.blockUntil = entry.cooldownFloor
	}
	s.entries[key] = entry
}

// claimCooldownPersistence 每个账号模型只允许一个写入者；其余请求不等待数据库。
func (s *openAIAccountModelTransientState) claimCooldownPersistence(accountID int64, model string, now time.Time) (time.Time, bool) {
	key, ok := openAIAccountModelTransientKey(accountID, model)
	if s == nil || !ok {
		return time.Time{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, exists := s.entries[key]
	if !exists || !entry.cooldownPersistingUntil.IsZero() || now.Before(entry.cooldownPersistRetryAt) ||
		!entry.cooldownFloor.After(now) || !entry.cooldownFloor.After(entry.cooldownPersistedUntil) {
		return time.Time{}, false
	}
	entry.cooldownPersistingUntil = entry.cooldownFloor
	s.entries[key] = entry
	return entry.cooldownPersistingUntil, true
}

// finishCooldownPersistence 合并写入期间的延长期限，失败时短暂退避以抑制重试风暴。
// 数据库操作始终在锁外进行，不同账号模型之间没有共享事务或等待链。
func (s *openAIAccountModelTransientState) finishCooldownPersistence(accountID int64, model string, until, now time.Time, succeeded bool) (time.Time, bool) {
	key, ok := openAIAccountModelTransientKey(accountID, model)
	if s == nil || !ok {
		return time.Time{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, exists := s.entries[key]
	if !exists || !entry.cooldownPersistingUntil.Equal(until) {
		return time.Time{}, false
	}
	entry.cooldownPersistingUntil = time.Time{}
	if !succeeded {
		entry.cooldownPersistRetryAt = now.Add(openAIModelCooldownPersistBackoff)
		s.entries[key] = entry
		return time.Time{}, false
	}
	if until.After(entry.cooldownPersistedUntil) {
		entry.cooldownPersistedUntil = until
	}
	entry.cooldownPersistRetryAt = time.Time{}
	if entry.cooldownFloor.After(now) && entry.cooldownFloor.After(entry.cooldownPersistedUntil) {
		entry.cooldownPersistingUntil = entry.cooldownFloor
	}
	s.entries[key] = entry
	return entry.cooldownPersistingUntil, !entry.cooldownPersistingUntil.IsZero()
}

func (s *openAIAccountModelTransientState) recordSuccess(accountID int64, model string) {
	key, ok := openAIAccountModelTransientKey(accountID, model)
	if s == nil || !ok {
		return
	}
	s.mu.Lock()
	if entry, exists := s.entries[key]; exists && time.Now().Before(entry.cooldownFloor) {
		entry.failureStreak = 0
		entry.blockUntil = entry.cooldownFloor
		s.entries[key] = entry
		s.mu.Unlock()
		return
	}
	delete(s.entries, key)
	s.mu.Unlock()
}

func (s *openAIAccountModelTransientState) isBlocked(accountID int64, model string, now time.Time) bool {
	key, ok := openAIAccountModelTransientKey(accountID, model)
	if s == nil || !ok {
		return false
	}
	if now.IsZero() {
		now = time.Now()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	entry, exists := s.entries[key]
	if !exists {
		return false
	}
	if !entry.lastFailure.IsZero() && now.Sub(entry.lastFailure) > openAIModelTransientStreakTTL && !now.Before(entry.cooldownFloor) {
		delete(s.entries, key)
		return false
	}
	entry.lastTouched = now
	s.entries[key] = entry
	return !entry.blockUntil.IsZero() && now.Before(entry.blockUntil)
}

func (s *openAIAccountModelTransientState) size() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.entries)
}

func (s *openAIAccountModelTransientState) evictOldestLocked() {
	if len(s.entries) < s.maxEntries {
		return
	}
	var oldestKey openAIAccountModelKey
	var oldestTime time.Time
	found := false
	for key, entry := range s.entries {
		if !found || entry.lastTouched.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.lastTouched
			found = true
		}
	}
	if found {
		delete(s.entries, oldestKey)
	}
}
