package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func TestCheckLoginPassword(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)
	require.NoError(t, err)
	stored := string(hash)
	stored2b := "$2b$" + stored[4:]
	stored2y := "$2y$" + stored[4:]
	otherHash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)
	require.NoError(t, err)
	require.NotEqual(t, stored, string(otherHash))
	invalidAlphabet := stored[:7] + strings.Repeat("!", 53)
	extraSuffix := stored + "x"
	truncated := stored[:len(stored)-1]
	invalidCost := stored[:4] + "03" + stored[6:]

	tests := []struct {
		name     string
		password string
		stored   string
		want     bool
	}{
		{name: "plaintext password", password: "correct-password", stored: stored, want: true},
		{name: "exact bcrypt value", password: stored, stored: stored, want: true},
		{name: "plaintext with 2b hash", password: "correct-password", stored: stored2b, want: true},
		{name: "exact 2b value", password: stored2b, stored: stored2b, want: true},
		{name: "exact 2y value", password: stored2y, stored: stored2y, want: true},
		{name: "modified bcrypt value", password: stored[:len(stored)-1] + "x", stored: stored, want: false},
		{name: "different salt for same plaintext", password: string(otherHash), stored: stored, want: false},
		{name: "wrong plaintext", password: "wrong-password", stored: stored, want: false},
		{name: "hash surrounded by whitespace", password: " " + stored + " ", stored: stored, want: false},
		{name: "invalid stored value", password: "invalid", stored: "invalid", want: false},
		{name: "invalid alphabet", password: invalidAlphabet, stored: invalidAlphabet, want: false},
		{name: "extra suffix", password: extraSuffix, stored: extraSuffix, want: false},
		{name: "truncated hash", password: truncated, stored: truncated, want: false},
		{name: "invalid cost", password: invalidCost, stored: invalidCost, want: false},
		{name: "empty password", password: "", stored: stored, want: false},
		{name: "empty stored value", password: "", stored: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, checkLoginPassword(tt.password, tt.stored))
		})
	}
}

func TestLoginHashDoesNotChangeGeneralPasswordChecks(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)
	require.NoError(t, err)
	stored := string(hash)
	svc := &AuthService{}
	user := &User{PasswordHash: stored}

	require.True(t, svc.CheckPassword("correct-password", stored))
	require.False(t, svc.CheckPassword(stored, stored))
	require.True(t, user.CheckPassword("correct-password"))
	require.False(t, user.CheckPassword(stored))
}
