package main

import (
	"context"
	"net/url"
	"strconv"
	"strings"
)

type relayGroupContextKey struct{}
type relayPayerContextKey struct{}
type relayCommissionContextKey struct{}

func withRelayGroup(ctx context.Context, groupID int64) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if groupID <= 0 {
		return ctx
	}
	return context.WithValue(ctx, relayGroupContextKey{}, groupID)
}

func relayGroupFromContext(ctx context.Context) int64 {
	if ctx == nil {
		return 0
	}
	groupID, _ := ctx.Value(relayGroupContextKey{}).(int64)
	return groupID
}

func stripClientGroupSelector(query url.Values) url.Values {
	if query == nil {
		return nil
	}
	cloned := url.Values{}
	for key, values := range query {
		if key == "group_id" {
			continue
		}
		cloned[key] = append([]string(nil), values...)
	}
	return cloned
}

func withRelayPayer(ctx context.Context, payerID string, commissionRate float64) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	payerID = strings.TrimSpace(payerID)
	if payerID == "" {
		return ctx
	}
	ctx = context.WithValue(ctx, relayPayerContextKey{}, payerID)
	if commissionRate > 0 && commissionRate < 1 {
		ctx = context.WithValue(ctx, relayCommissionContextKey{}, commissionRate)
	}
	return ctx
}

func relayPayerFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	payer, _ := ctx.Value(relayPayerContextKey{}).(string)
	return payer
}

func relayCommissionFromContext(ctx context.Context) float64 {
	if ctx == nil {
		return 0
	}
	rate, _ := ctx.Value(relayCommissionContextKey{}).(float64)
	return rate
}

func relayGroupHeader(groupID int64) string {
	if groupID <= 0 {
		return ""
	}
	return strconv.FormatInt(groupID, 10)
}
