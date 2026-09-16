package service

import "testing"

func TestAccountMediaProtocolProfileIsOptIn(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		want    string
		enabled bool
	}{
		{"nil", nil, "", false},
		{"missing", &Account{Extra: map[string]any{}}, "", false},
		{"unknown", &Account{Extra: map[string]any{MediaProtocolProfileExtraKey: "custom"}}, "", false},
		{"compatible", &Account{Extra: map[string]any{MediaProtocolProfileExtraKey: MediaProtocolProfileOpenAICompatible}}, MediaProtocolProfileOpenAICompatible, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, enabled := tt.account.MediaProtocolProfile()
			if got != tt.want || enabled != tt.enabled {
				t.Fatalf("got (%q,%v), want (%q,%v)", got, enabled, tt.want, tt.enabled)
			}
		})
	}
}
