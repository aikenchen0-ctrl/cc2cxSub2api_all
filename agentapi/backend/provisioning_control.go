package main

import (
	"context"
	"log/slog"
	"strings"
	"time"
)

// runProvisioningControl reads this Agent's authoritative desired state through
// its per-Agent management API credential. ID-only database notification SSE
// wakes an immediate GET; periodic GET remains the reconnect and stale-state
// fallback. The runtime process never connects directly to the main database.
func (s *Server) runProvisioningControl(ctx context.Context) {
	refresh := func() {
		if err := s.refreshProvisioningStatus(ctx); err != nil {
			slog.Warn("main-site agent status refresh failed", "agent_id", s.cfg.AgentID, "error", err)
		}
	}
	refresh()
	interval := s.cfg.ProvisioningControlStaleAfter / 3
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if interval < 2*time.Second {
		interval = 2 * time.Second
	}
	if interval > 30*time.Second {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	reconnect := time.NewTimer(0)
	defer reconnect.Stop()
	var updates <-chan struct{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			refresh()
		case <-reconnect.C:
			stream, err := s.main.RuntimeAgentUpdates(ctx)
			if err != nil {
				slog.Warn("main-site agent status stream unavailable; periodic refresh remains active", "agent_id", s.cfg.AgentID, "error", err)
				reconnect.Reset(5 * time.Second)
				continue
			}
			updates = stream
		case _, open := <-updates:
			if !open {
				updates = nil
				reconnect.Reset(5 * time.Second)
				continue
			}
			refresh()
		}
	}
}

func (s *Server) refreshProvisioningStatus(ctx context.Context) error {
	requestCtx, cancel := contextWithMainTimeout(ctx, s.cfg)
	defer cancel()
	agent, err := s.main.RuntimeGetProvisioningAgent(requestCtx, s.cfg.AgentID)
	if err != nil {
		return err
	}
	s.setProvisioningState(agent.Status, time.Now().UTC())
	return nil
}

func (s *Server) setProvisioningState(status string, checkedAt time.Time) {
	s.provisioningMu.Lock()
	s.provisioningStatus = strings.TrimSpace(status)
	s.provisioningCheckedAt = checkedAt
	s.provisioningMu.Unlock()
}

func (s *Server) provisioningState(now time.Time) (string, bool) {
	if !s.cfg.ProvisioningControlEnabled {
		return "active", true
	}
	s.provisioningMu.RLock()
	status, checkedAt := s.provisioningStatus, s.provisioningCheckedAt
	s.provisioningMu.RUnlock()
	staleAfter := s.cfg.ProvisioningControlStaleAfter
	if staleAfter <= 0 {
		staleAfter = 90 * time.Second
	}
	if status == "" || checkedAt.IsZero() || now.Sub(checkedAt) > staleAfter {
		return "", false
	}
	return status, true
}
