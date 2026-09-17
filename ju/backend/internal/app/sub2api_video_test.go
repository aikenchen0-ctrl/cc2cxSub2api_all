package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"infinite-canvas/backend/internal/model"
)

func TestSub2APIVideoCreateResumeAndAuthenticatedDownload(t *testing.T) {
	adapter, ok := loadOfficialFallbackRegistry().Resolve(sub2APIVideoProtocol)
	if !ok {
		t.Fatal("Sub2API plugin not installed")
	}
	creates, polls, downloads := 0, 0, 0
	retrying := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-server-key" {
			t.Error("server credential missing")
		}
		switch r.Method + " " + r.URL.Path {
		case "POST /v1/videos":
			creates++
			if r.Header.Get("Content-Type") != "application/json" {
				t.Error("create must use JSON")
			}
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Error(err)
			}
			if payload["model"] != "minimax_h3_b99_001" || payload["duration"] != float64(5) {
				t.Errorf("payload = %#v", payload)
			}
			_, _ = w.Write([]byte(`{"code":"Success","data":{"task_id":"job","status":"queued"}}`))
		case "GET /v1/videos/job":
			polls++
			if !retrying {
				http.Error(w, "temporarily unavailable", 503)
				return
			}
			_, _ = w.Write([]byte(`{"code":"Success","data":{"task_id":"job","status":"completed","results":[{"url":"https://never-fetch.example/a.mp4"}]}}`))
		case "GET /v1/videos/job/content":
			downloads++
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("test-video"))
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	config := providerConfig{BaseURL: server.URL + "/v1", APIKey: "test-server-key", Model: "minimax_h3_b99_001", InterfaceType: sub2APIVideoProtocol, VideoSeconds: "5", VQuality: "736p", Size: "16:9", AllowLocalChannel: true}
	input := canvasGenerationInput{Mode: "video", Prompt: "scene", Config: config}
	ctx := withProviderOutboundPolicy(context.Background(), config)
	_, err := runProtocolAdapterTaskWithTiming(ctx, input, adapter, protocolPollTiming{PollInterval: time.Millisecond})
	var pending providerStatePendingError
	if !errors.As(err, &pending) || pending.TaskID != "job" {
		t.Fatalf("expected resumable error, got %v", err)
	}
	retrying = true
	ctx = withProviderAnalytics(ctx, nil, model.Task{ID: "local-task", Type: "canvas_video", ProviderRequestID: pending.TaskID})
	result, err := runVideoTask(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if result["mode"] != "video" || creates != 1 || polls != 2 || downloads != 1 {
		t.Fatalf("result=%#v calls=%d/%d/%d", result, creates, polls, downloads)
	}
	encoded, _ := json.Marshal(result)
	if strings.Contains(string(encoded), "test-server-key") || strings.Contains(string(encoded), "never-fetch") {
		t.Fatal("provider secret/URL leaked")
	}
}

func TestSub2APIVideoRejectsNonVideoContentAndWrongTask(t *testing.T) {
	for _, scenario := range []string{"content", "task"} {
		t.Run(scenario, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if strings.HasSuffix(r.URL.Path, "/content") {
					_, _ = w.Write([]byte(`{"error":"not ready"}`))
					return
				}
				id := "job"
				if scenario == "task" {
					id = "other-job"
				}
				_, _ = w.Write([]byte(`{"id":"` + id + `","status":"completed"}`))
			}))
			defer server.Close()
			config := providerConfig{BaseURL: server.URL + "/v1", APIKey: "key", Model: "minimax_h3_b99_001", InterfaceType: sub2APIVideoProtocol, AllowLocalChannel: true}
			svc, db := newChannelModelTestService(t)
			task := model.Task{ID: "local-task", Type: "canvas_video", ProviderRequestID: "job"}
			if err := db.Create(&task).Error; err != nil {
				t.Fatal(err)
			}
			ctx := withProviderOutboundPolicy(context.Background(), config)
			ctx = withProviderAnalytics(ctx, svc, task)
			if _, err := runVideoTask(ctx, canvasGenerationInput{Mode: "video", Config: config}); err == nil {
				t.Fatal("invalid response accepted")
			}
			adapter, _ := loadOfficialFallbackRegistry().Resolve(sub2APIVideoProtocol)
			if _, _, err := queryProtocolAdapterVideoTask(ctx, canvasGenerationInput{Mode: "video", Config: config}, adapter, "job"); err == nil {
				t.Fatal("manual recovery accepted an invalid response")
			}
			stored, err := svc.repo.Task(task.ID)
			if err != nil || stored.ProviderRequestID != "job" {
				t.Fatalf("audit changed accepted task ID: %#v, err=%v", stored, err)
			}
		})
	}
}

func TestSub2APIVideoInterruptedSubmissionDoesNotResubmit(t *testing.T) {
	svc, db := newChannelModelTestService(t)
	task := model.Task{ID: "interrupted", Type: "canvas_video", Status: model.TaskStatusRunning}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	ctx := withProviderAnalytics(context.Background(), svc, task)
	if err := beginSub2APIVideoSubmission(ctx); err != nil {
		t.Fatal(err)
	}
	// Simulate a process loss after dispatch but before the accepted ID is saved.
	claimed, err := svc.repo.ClaimNextTask("replacement-worker", time.Minute)
	if err != nil || claimed == nil || claimed.PollStage != "submission_unknown" {
		t.Fatalf("reclaim lost submission fence: %#v, err=%v", claimed, err)
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.Error(w, "must not resubmit", 500)
	}))
	defer server.Close()
	config := providerConfig{BaseURL: server.URL + "/v1", APIKey: "test-key", Model: "minimax_h3_b99_001", InterfaceType: sub2APIVideoProtocol, AllowLocalChannel: true}
	ctx = withProviderOutboundPolicy(withProviderAnalytics(context.Background(), svc, *claimed), config)
	_, err = runVideoTask(ctx, canvasGenerationInput{Mode: "video", Prompt: "scene", Config: config})
	if !isRouteDispatchUncertain(err) || requests != 0 {
		t.Fatalf("uncertain submission was retried: requests=%d err=%v", requests, err)
	}
}

func TestSub2APIVideoSubmissionFencePersistsThroughAudit(t *testing.T) {
	svc, db := newChannelModelTestService(t)
	task := model.Task{ID: "ambiguous", Type: "canvas_video"}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	ctx := withProviderAnalytics(context.Background(), svc, task)
	if err := beginSub2APIVideoSubmission(ctx); err != nil {
		t.Fatal(err)
	}
	// A response without an ID can update the log stage but must not reopen POST.
	_ = svc.LogAPICall(model.ApiCallLog{TaskID: task.ID, RequestKind: "create", Status: model.ApiCallStatusFailed})
	if err := beginSub2APIVideoSubmission(ctx); !isRouteDispatchUncertain(err) {
		t.Fatalf("audit reopened uncertain submission: %v", err)
	}
}

func TestSub2APIVideoAcceptedIDIsDurableAfterCancellation(t *testing.T) {
	svc, db := newChannelModelTestService(t)
	task := model.Task{ID: "accepted", Type: "canvas_video"}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(withProviderAnalytics(context.Background(), svc, task))
	cancel()
	if err := persistSub2APIVideoRequest(ctx, "provider-job"); err != nil {
		t.Fatal(err)
	}
	stored, err := svc.repo.Task(task.ID)
	if err != nil || stored.ProviderRequestID != "provider-job" {
		t.Fatalf("stored=%#v err=%v", stored, err)
	}
}

func TestSub2APIVideoRecoveryWindow(t *testing.T) {
	svc := &Service{}
	started := time.Now().Add(-time.Hour)
	task := model.Task{ID: "task", Type: "canvas_video", ProviderRequestID: "job", StartedAt: &started}
	input, _ := json.Marshal(canvasGenerationInput{Config: providerConfig{BaseURL: "https://93.184.216.34/v1", APIKey: "key", Model: "minimax_h3_b99_001", InterfaceType: sub2APIVideoProtocol}})
	pending := providerStatePendingError{TaskID: "job", Cause: providerHTTPError{StatusCode: 503}}
	if !svc.shouldDeferVideoProviderTask(task, string(input), pending) {
		t.Fatal("transient poll error must resume the accepted task")
	}
	if !svc.shouldDeferVideoProviderTask(task, string(input), context.DeadlineExceeded) {
		t.Fatal("timeout must resume the accepted task")
	}
	if svc.shouldDeferVideoProviderTask(task, string(input), errors.New("invalid request")) {
		t.Fatal("permanent errors must not be deferred")
	}
	expired := time.Now().Add(-25 * time.Hour)
	task.StartedAt = &expired
	if svc.shouldDeferVideoProviderTask(task, string(input), pending) {
		t.Fatal("must bound automatic polling")
	}
}
