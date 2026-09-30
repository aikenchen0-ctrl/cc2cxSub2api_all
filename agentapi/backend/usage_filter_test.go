package main

import (
	"net/url"
	"testing"
)

func TestUsageFilterParsing(t *testing.T) {
	for _, raw := range []string{"start_time=bad", "start_time=2026-09-29T00:00:00Z&end_time=2026-09-28T00:00:00Z", "end_time=1970-01-01T00:00:00Z"} {
		q, _ := url.ParseQuery(raw)
		if _, err := parseUsageFilter(q); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	f, err := parseUsageFilter(url.Values{"start_time": {"2026-09-29T08:00:00+08:00"}, "end_time": {"2026-09-30T00:00:00Z"}, "model": {" model-a "}})
	if err != nil || f.Model != "model-a" || f.End-f.Start != 86400 {
		t.Fatalf("filter=%+v err=%v", f, err)
	}
}

func TestFilteredUsagePageKeepsScopeCountAndExclusiveEnd(t *testing.T) {
	s := testStore(t)
	for _, row := range []struct {
		agent, user, request, model string
		created                     int64
	}{
		{"agent-test", "42", "a", "m", 100},
		{"agent-test", "42", "b", "m", 101},
		{"agent-test", "42", "c", "m", 102},
		{"agent-test", "42", "d", "other", 101},
		{"agent-test", "43", "e", "m", 101},
		{"another-agent", "42", "f", "m", 101},
	} {
		_, err := s.db.Exec(`INSERT INTO settlements (agent_id, proxy_main_user_id, billing_main_user_id, request_id, model, created_at, updated_at, usage_id, reserved_cents) VALUES (?, ?, ?, ?, ?, ?, ?, '', 0)`, row.agent, row.user, row.user, row.request, row.model, row.created, row.created)
		if err != nil {
			t.Fatal(err)
		}
	}
	rows, total, err := s.FilteredUsagePage("agent-test", "42", 1, 1, UsageFilter{Model: "m", Start: 100, End: 102})
	if err != nil || total != 2 || len(rows) != 1 || rows[0].RequestID != "a" {
		t.Fatalf("rows=%+v total=%d err=%v", rows, total, err)
	}
	_, total, err = s.FilteredUsagePage("agent-test", "", 10, 0, UsageFilter{Model: "m", Start: 100, End: 102})
	if err != nil || total != 3 {
		t.Fatalf("admin count=%d err=%v", total, err)
	}
	for _, request := range []string{"e", "' OR 1=1 --"} {
		_, total, err = s.FilteredUsagePage("agent-test", "42", 10, 0, UsageFilter{RequestID: request})
		if err != nil || total != 0 {
			t.Fatalf("request escaped scope: count=%d err=%v", total, err)
		}
	}
}
