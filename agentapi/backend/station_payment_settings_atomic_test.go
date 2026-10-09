package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStationPaymentSettingsAndBillingModeCommitTogether(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	defer upstream.Close()
	s := testServer(t, upstream).store
	settings := defaultStationPaymentSettings()
	if err := s.SaveStationPaymentSettings("agent-test", settings); err != nil {
		t.Fatal(err)
	}
	_, err := s.db.Exec(`CREATE TRIGGER reject_billing_update BEFORE UPDATE OF billing_mode ON agent_config BEGIN SELECT RAISE(ABORT,'simulated mode write failure'); END`)
	if err != nil {
		t.Fatal(err)
	}
	settings.Enabled = true
	settings.MinAmount = 22
	if err := s.SaveStationPaymentSettings("agent-test", settings); err == nil {
		t.Fatal("expected transaction failure")
	}
	got, err := s.StationPaymentSettings("agent-test")
	if err != nil || got.Enabled || got.MinAmount == 22 {
		t.Fatalf("partial settings commit: %+v err=%v", got, err)
	}
	agent, err := s.Agent("agent-test")
	if err != nil || agent.BillingMode != "user_upstream" {
		t.Fatalf("partial mode commit: %+v err=%v", agent, err)
	}
	if _, err := s.db.Exec(`DROP TRIGGER reject_billing_update`); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveStationPaymentSettings("agent-test", settings); err != nil {
		t.Fatal(err)
	}
	agent, err = s.Agent("agent-test")
	if err != nil || agent.BillingMode != "owner_upstream" || agent.OwnerMainUserID != "42" {
		t.Fatalf("enabled mode: %+v err=%v", agent, err)
	}
}
