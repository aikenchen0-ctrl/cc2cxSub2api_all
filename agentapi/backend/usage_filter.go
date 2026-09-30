package main

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Bounds are UTC instants, start inclusive and end exclusive.
func parseUsageFilter(q url.Values) (UsageFilter, error) {
	f := UsageFilter{Model: strings.TrimSpace(q.Get("model")), RequestID: strings.TrimSpace(q.Get("request_id"))}
	if len(f.Model) > 256 || len(f.RequestID) > 256 {
		return f, fmt.Errorf("model and request_id must not exceed 256 bytes")
	}
	for _, bound := range []struct {
		key    string
		target *int64
	}{{"start_time", &f.Start}, {"end_time", &f.End}} {
		if raw := q.Get(bound.key); raw != "" {
			parsed, err := time.Parse(time.RFC3339, raw)
			if err != nil || parsed.Unix() <= 0 {
				return f, fmt.Errorf("%s must be a valid RFC3339 time after Unix epoch", bound.key)
			}
			*bound.target = parsed.Unix()
		}
	}
	if f.Start != 0 && f.End != 0 && f.Start >= f.End {
		return f, fmt.Errorf("start_time must be before end_time")
	}
	return f, nil
}
