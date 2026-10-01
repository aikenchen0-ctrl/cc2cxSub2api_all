package service

import (
	"context"
	"testing"
	"time"
)

type agentTenantRepoStub struct {
	tenant *AgentSharedTenant
	calls  int
}

func (r *agentTenantRepoStub) EnsureSharedTenant(_ context.Context, tenant AgentSharedTenant) (*AgentSharedTenant, error) {
	r.calls++
	if r.tenant == nil {
		copy := tenant
		now := time.Now().UTC()
		copy.CreatedAt, copy.UpdatedAt = now, now
		r.tenant = &copy
	}
	copy := *r.tenant
	return &copy, nil
}

func TestEnsureSharedTenantIsIdempotentPerOwner(t *testing.T) {
	repo := &agentTenantRepoStub{}
	svc := NewAgentTenantService(repo)
	first, err := svc.EnsureSharedTenant(context.Background(), 42, " Agent Owner ")
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.EnsureSharedTenant(context.Background(), 42, "Changed Name")
	if err != nil {
		t.Fatal(err)
	}
	if first.AgentID == "" || second.AgentID != first.AgentID || repo.calls != 2 {
		t.Fatalf("shared tenant was not stable: first=%+v second=%+v calls=%d", first, second, repo.calls)
	}
	if first.OwnerMainUserID != 42 || first.Role != "owner" || first.Status != "active" || first.DisplayName != "Agent Owner" {
		t.Fatalf("unexpected shared tenant: %+v", first)
	}
}

func TestEnsureSharedTenantRejectsInvalidOwnerAndUnsafeName(t *testing.T) {
	svc := NewAgentTenantService(&agentTenantRepoStub{})
	if _, err := svc.EnsureSharedTenant(context.Background(), 0, "owner"); err != ErrAgentTenantInvalid {
		t.Fatalf("invalid owner error = %v", err)
	}
	if _, err := svc.EnsureSharedTenant(context.Background(), 1, "unsafe\nname"); err != ErrAgentTenantInvalid {
		t.Fatalf("unsafe display name error = %v", err)
	}
}
