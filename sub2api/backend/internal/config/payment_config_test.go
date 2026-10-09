package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadPaymentOrderExpiryEnabled(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		env  string
		want bool
	}{
		{name: "default", want: true},
		{name: "yaml_disabled", yaml: "payment:\n  order_expiry_enabled: false\n", want: false},
		{name: "env_disabled", env: "false", want: false},
		{name: "env_overrides_yaml", yaml: "payment:\n  order_expiry_enabled: true\n", env: "false", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetViperWithJWTSecret(t)
			t.Setenv("PAYMENT_ORDER_EXPIRY_ENABLED", tt.env)
			if tt.yaml != "" {
				path := filepath.Join(t.TempDir(), "config.yaml")
				require.NoError(t, os.WriteFile(path, []byte(tt.yaml), 0o600))
				t.Setenv("CONFIG_FILE", path)
			}
			cfg, err := Load()
			require.NoError(t, err)
			require.Equal(t, tt.want, cfg.Payment.OrderExpiryEnabled)
		})
	}
}
