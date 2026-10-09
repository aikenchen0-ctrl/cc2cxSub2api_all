package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type AgentKeyGroup struct {
	ID             int64   `json:"id"`
	Name           string  `json:"name"`
	Platform       string  `json:"platform"`
	Description    string  `json:"description,omitempty"`
	Status         string  `json:"status"`
	RateMultiplier float64 `json:"rate_multiplier"`
}

func (c *MainClient) AdminListKeyGroups(ctx context.Context) ([]AgentKeyGroup, error) {
	data, err := c.adminGet(ctx, "/admin/groups/all")
	if err != nil {
		return nil, err
	}
	var raw []map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, &MainAPIError{Status: http.StatusBadGateway, Code: "MAIN_ADMIN_UNAVAILABLE", Message: "main-site group lookup returned an invalid response"}
	}
	groups := make([]AgentKeyGroup, 0, len(raw))
	for _, item := range raw {
		group := AgentKeyGroup{
			ID: int64(fieldFloat(item, "id")), Name: fieldString(item, "name"), Platform: fieldString(item, "platform"),
			Description: fieldString(item, "description"), Status: fieldString(item, "status"), RateMultiplier: fieldFloat(item, "rate_multiplier"),
		}
		if group.ID <= 0 || group.Name == "" || !strings.EqualFold(group.Status, "active") {
			continue
		}
		groups = append(groups, group)
	}
	return groups, nil
}

func (c *MainClient) CreateUserGroupKey(ctx context.Context, mainUserID, name string, groupID int64) error {
	_, err := c.satelliteUserMutationJSON(ctx, mainUserID, "/v1/sub2api/api-keys", map[string]any{
		"name": name, "group_id": groupID,
	})
	return err
}
