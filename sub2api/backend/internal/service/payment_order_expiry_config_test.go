//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type paymentExpiryStartProbe struct {
	attempts chan string
}

func (p *paymentExpiryStartProbe) TryAcquireLeaderLock(_ context.Context, key, _ string, _ time.Duration) (bool, error) {
	p.attempts <- key
	return false, nil
}

func (p *paymentExpiryStartProbe) ReleaseLeaderLock(context.Context, string, string) error {
	return nil
}

func TestProvidePaymentOrderExpiryServiceEnabled(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Config
		want int
	}{
		{name: "nil_config_default", want: 1},
		{name: "enabled", cfg: &config.Config{Payment: config.PaymentConfig{OrderExpiryEnabled: true}}, want: 1},
		{name: "disabled", cfg: &config.Config{Payment: config.PaymentConfig{OrderExpiryEnabled: false}}, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			probe := &paymentExpiryStartProbe{attempts: make(chan string, 1)}
			svc := ProvidePaymentOrderExpiryService(&PaymentService{}, probe, nil, tt.cfg)
			svc.Stop()
			require.Len(t, probe.attempts, tt.want)
			if tt.want > 0 {
				require.Equal(t, paymentOrderExpiryLeaderLockKey, <-probe.attempts)
			}
		})
	}
}
