package main

import (
	"net/http"
)

// AgentAdminPromoCodesView preserves the main-site promo-code management
// contract without inventing an unsafe financial implementation. Promo codes
// credit authoritative Sub2API balances during registration, so AgentAPI must
// not enable issuance until a tenant-funded, atomic debit/credit protocol has
// been configured on the main site.
type AgentAdminPromoCodesView struct {
	FeatureEnabled    bool                  `json:"feature_enabled"`
	FundingMode       string                `json:"funding_mode"`
	Authority         string                `json:"authority"`
	ManagementScope   string                `json:"management_scope"`
	RedemptionEnabled bool                  `json:"redemption_enabled"`
	CanCreate         bool                  `json:"can_create"`
	CanEdit           bool                  `json:"can_edit"`
	CanDelete         bool                  `json:"can_delete"`
	Items             []AgentAdminPromoCode `json:"items"`
	Total             int                   `json:"total"`
}

type AgentAdminPromoCode struct {
	Code        string  `json:"code"`
	BonusAmount float64 `json:"bonus_amount"`
	MaxUses     int     `json:"max_uses"`
	UsedCount   int     `json:"used_count"`
	Status      string  `json:"status"`
	ExpiresAt   *string `json:"expires_at"`
	CreatedAt   string  `json:"created_at"`
}

func (s *Server) handleAgentAdminPromoCodes(w http.ResponseWriter, r *http.Request, requestID string) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "promo-code issuance requires a configured main-site funding protocol")
		return
	}
	s.writeData(w, http.StatusOK, requestID, AgentAdminPromoCodesView{
		FeatureEnabled:    false,
		FundingMode:       "unconfigured",
		Authority:         "sub2api_main",
		ManagementScope:   "current_agent",
		RedemptionEnabled: false,
		CanCreate:         false,
		CanEdit:           false,
		CanDelete:         false,
		Items:             []AgentAdminPromoCode{},
		Total:             0,
	})
}
