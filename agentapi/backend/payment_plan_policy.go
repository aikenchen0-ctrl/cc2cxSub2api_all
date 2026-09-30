package main

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

var errAgentPlanDisabled = errors.New("subscription plan is hidden on this agent")

type AgentAdminCheckoutPlan struct {
	AgentCheckoutPlan
	Enabled           bool     `json:"enabled"`
	SortOrder         int      `json:"sort_order"`
	Customized        bool     `json:"customized"`
	SourceName        string   `json:"source_name"`
	SourceDescription string   `json:"source_description"`
	SourceFeatures    []string `json:"source_features"`
}

type agentPlanPolicyInput struct {
	Enabled     *bool    `json:"enabled"`
	SortOrder   int      `json:"sort_order"`
	DisplayName string   `json:"display_name"`
	Description string   `json:"description"`
	Features    []string `json:"features"`
}

func normalizeAgentPlanPolicyInput(input agentPlanPolicyInput) (AgentPlanPolicy, error) {
	if input.Enabled == nil {
		return AgentPlanPolicy{}, errors.New("enabled is required")
	}
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Description = strings.TrimSpace(input.Description)
	if len(input.DisplayName) > 120 {
		return AgentPlanPolicy{}, errors.New("display_name is too long")
	}
	if len(input.Description) > 2000 {
		return AgentPlanPolicy{}, errors.New("description is too long")
	}
	if input.SortOrder < 0 || input.SortOrder > 100000 {
		return AgentPlanPolicy{}, errors.New("sort_order must be between 0 and 100000")
	}
	if len(input.Features) > 20 {
		return AgentPlanPolicy{}, errors.New("features cannot contain more than 20 items")
	}
	features := make([]string, 0, len(input.Features))
	for _, feature := range input.Features {
		feature = strings.TrimSpace(feature)
		if feature == "" {
			continue
		}
		if len(feature) > 200 {
			return AgentPlanPolicy{}, errors.New("a feature is too long")
		}
		features = append(features, feature)
	}
	return AgentPlanPolicy{
		Enabled: *input.Enabled, SortOrder: input.SortOrder,
		DisplayName: input.DisplayName, Description: input.Description, Features: features,
	}, nil
}

func (s *Server) applyAgentPlanPolicies(checkout AgentCheckoutInfo) (AgentCheckoutInfo, error) {
	policies, err := s.store.AgentPlanPolicies(s.cfg.AgentID)
	if err != nil {
		return AgentCheckoutInfo{}, err
	}
	byID := make(map[int64]AgentPlanPolicy, len(policies))
	for _, policy := range policies {
		byID[policy.PlanID] = policy
	}
	type orderedPlan struct {
		plan  AgentCheckoutPlan
		order int
		index int
	}
	visible := make([]orderedPlan, 0, len(checkout.Plans))
	for index, source := range checkout.Plans {
		policy, customized := byID[source.ID]
		if customized && !policy.Enabled {
			continue
		}
		plan := source
		order := index
		if customized {
			order = policy.SortOrder
			applyAgentPlanDisplayPolicy(&plan, policy)
		}
		visible = append(visible, orderedPlan{plan: plan, order: order, index: index})
	}
	sort.SliceStable(visible, func(i, j int) bool {
		if visible[i].order != visible[j].order {
			return visible[i].order < visible[j].order
		}
		return visible[i].index < visible[j].index
	})
	checkout.Plans = make([]AgentCheckoutPlan, 0, len(visible))
	for _, item := range visible {
		checkout.Plans = append(checkout.Plans, item.plan)
	}
	return checkout, nil
}

func applyAgentPlanDisplayPolicy(plan *AgentCheckoutPlan, policy AgentPlanPolicy) {
	if policy.DisplayName != "" {
		plan.Name = policy.DisplayName
	}
	if policy.Description != "" {
		plan.Description = policy.Description
	}
	if len(policy.Features) > 0 {
		plan.Features = append([]string(nil), policy.Features...)
	}
}

func (s *Server) ensureAgentPlanEnabled(planID int64) error {
	policy, err := s.store.AgentPlanPolicy(s.cfg.AgentID, planID)
	if errors.Is(err, errNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !policy.Enabled {
		return errAgentPlanDisabled
	}
	return nil
}

func mergeAgentAdminPlans(plans []AgentCheckoutPlan, policies []AgentPlanPolicy) []AgentAdminCheckoutPlan {
	byID := make(map[int64]AgentPlanPolicy, len(policies))
	for _, policy := range policies {
		byID[policy.PlanID] = policy
	}
	items := make([]AgentAdminCheckoutPlan, 0, len(plans))
	for index, source := range plans {
		policy, customized := byID[source.ID]
		effective := source
		enabled, order := true, index
		if customized {
			enabled, order = policy.Enabled, policy.SortOrder
			applyAgentPlanDisplayPolicy(&effective, policy)
		}
		items = append(items, AgentAdminCheckoutPlan{
			AgentCheckoutPlan: effective,
			Enabled:           enabled, SortOrder: order, Customized: customized,
			SourceName: source.Name, SourceDescription: source.Description,
			SourceFeatures: append([]string(nil), source.Features...),
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].SortOrder != items[j].SortOrder {
			return items[i].SortOrder < items[j].SortOrder
		}
		return items[i].ID < items[j].ID
	})
	return items
}

func findAgentCheckoutPlan(plans []AgentCheckoutPlan, planID int64) (AgentCheckoutPlan, bool) {
	for _, plan := range plans {
		if plan.ID == planID {
			return plan, true
		}
	}
	return AgentCheckoutPlan{}, false
}

func (s *Server) handleAgentAdminPaymentPlans(w http.ResponseWriter, r *http.Request, requestID string, actor Session) {
	const basePath = "/api/v1/agent/admin/payment/plans"
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.Path == basePath {
		if r.Method != http.MethodGet {
			s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		checkout, err := s.main.PaymentCheckoutInfo(r.Context(), actor.MainUserID)
		if err != nil {
			s.writeMainError(w, requestID, err)
			return
		}
		policies, err := s.store.AgentPlanPolicies(s.cfg.AgentID)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load agent plan policies")
			return
		}
		s.writeData(w, http.StatusOK, requestID, map[string]any{"items": mergeAgentAdminPlans(checkout.Plans, policies), "total": len(checkout.Plans)})
		return
	}

	relative := strings.TrimPrefix(r.URL.Path, basePath+"/")
	planID, err := strconv.ParseInt(relative, 10, 64)
	if err != nil || planID <= 0 || strings.Contains(relative, "/") {
		s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "not found")
		return
	}
	if r.Method != http.MethodPut {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	if !sameOrigin(r) {
		s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin state-changing requests are not allowed")
		return
	}
	checkout, err := s.main.PaymentCheckoutInfo(r.Context(), actor.MainUserID)
	if err != nil {
		s.writeMainError(w, requestID, err)
		return
	}
	source, ok := findAgentCheckoutPlan(checkout.Plans, planID)
	if !ok {
		s.writeError(w, http.StatusNotFound, requestID, "MAIN_PLAN_NOT_FOUND", "main-site subscription plan not found")
		return
	}
	var input agentPlanPolicyInput
	if err := decodeJSON(r, &input, 64<<10); err != nil {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", "invalid agent plan policy")
		return
	}
	policy, err := normalizeAgentPlanPolicyInput(input)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_PLAN_POLICY", err.Error())
		return
	}
	policy.PlanID = planID
	policy, err = s.store.UpsertAgentPlanPolicy(s.cfg.AgentID, policy)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to save agent plan policy")
		return
	}
	effective := source
	applyAgentPlanDisplayPolicy(&effective, policy)
	result := AgentAdminCheckoutPlan{
		AgentCheckoutPlan: effective,
		Enabled:           policy.Enabled, SortOrder: policy.SortOrder, Customized: true,
		SourceName: source.Name, SourceDescription: source.Description,
		SourceFeatures: append([]string(nil), source.Features...),
	}
	s.recordAudit("agent_admin", actor.MainUserID, "payment_plan_policy.update", "main_plan", strconv.FormatInt(planID, 10), requestID, "success", "")
	s.writeData(w, http.StatusOK, requestID, result)
}
