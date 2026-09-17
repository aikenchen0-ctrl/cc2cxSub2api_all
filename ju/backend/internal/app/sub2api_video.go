package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/repository"
)

const sub2APIVideoProtocol = "sub2api-video-v1"
const sub2APIVideoRecoveryMaxAge = 24 * time.Hour

type sub2APIVideoRequestKey struct{}

func beginSub2APIVideoSubmission(ctx context.Context) error {
	metadata, ok := ctx.Value(providerAnalyticsKey{}).(providerAnalyticsContext)
	if !ok || metadata.Service == nil || metadata.TaskID == "" {
		return nil
	}
	if err := metadata.Service.repo.WithContext(ctx).BeginTaskProviderSubmission(metadata.TaskID); err != nil {
		if errors.Is(err, repository.ErrTaskStateConflict) {
			return routeDispatchUncertainError{"Sub2API submission may already have been sent; automatic resubmission stopped to avoid duplicate charges"}
		}
		return fmt.Errorf("cannot persist Sub2API submission state: %w", err)
	}
	return nil
}

func withSub2APIVideoTaskID(ctx context.Context, taskID string) context.Context {
	metadata, _ := ctx.Value(providerAnalyticsKey{}).(providerAnalyticsContext)
	metadata.ProviderRequestID = taskID
	return context.WithValue(ctx, providerAnalyticsKey{}, metadata)
}

// Audit enrichment runs before adapter validation. It must not replace the
// accepted task ID with an untrusted poll response ID.
func protectSub2APIVideoLog(ctx context.Context, entry *model.ApiCallLog) {
	if enabled, _ := ctx.Value(sub2APIVideoRequestKey{}).(bool); !enabled {
		return
	}
	if entry.RequestKind == "poll" || entry.RequestKind == "download" {
		entry.ProviderRequestID = resumedProviderRequestID(ctx)
	}
}

// Accepted jobs must be durable before polling or downloading, independently
// of best-effort API logs. Cancellation after acceptance must not skip this write.
func persistSub2APIVideoRequest(ctx context.Context, requestID string) error {
	metadata, ok := ctx.Value(providerAnalyticsKey{}).(providerAnalyticsContext)
	if !ok || metadata.Service == nil || metadata.TaskID == "" {
		return nil
	}
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := metadata.Service.repo.WithContext(persistCtx).UpdateTaskProviderState(metadata.TaskID, requestID, "accepted", nil); err != nil {
		return fmt.Errorf("Sub2API accepted video task %s but its ID could not be persisted; do not resubmit automatically: %w", requestID, err)
	}
	return nil
}

func retryableSub2APIVideoError(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var httpErr providerHTTPError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode == 429 || httpErr.StatusCode >= 500
	}
	var networkErr net.Error
	return errors.As(err, &networkErr) || retryableProtocolMediaDownload(err)
}
