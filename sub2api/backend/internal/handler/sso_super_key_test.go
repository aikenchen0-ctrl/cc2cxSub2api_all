//go:build unit

package handler

import "testing"

func withTestSuperKey(t *testing.T) {
	t.Helper()
	// SSO tickets are identity-only; Super Key is resolved on /v1, not in the ticket.
}
