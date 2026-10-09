//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func TestAuthServiceLoginStoredBcrypt(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("login-password"), bcrypt.MinCost)
	require.NoError(t, err)
	stored := string(hash)
	stored2b := "$2b$" + stored[4:]
	otherHash, err := bcrypt.GenerateFromPassword([]byte("login-password"), bcrypt.MinCost)
	require.NoError(t, err)

	tests := []struct {
		name        string
		password    string
		stored      string
		email       string
		status      string
		expectedErr error
	}{
		{name: "plaintext", password: "login-password", stored: stored, status: StatusActive},
		{name: "stored 2a hash", password: stored, stored: stored, status: StatusActive},
		{name: "stored 2b hash", password: stored2b, stored: stored2b, status: StatusActive},
		{name: "different hash of same password", password: string(otherHash), stored: stored, status: StatusActive, expectedErr: ErrInvalidCredentials},
		{name: "wrong plaintext", password: "wrong-password", stored: stored, status: StatusActive, expectedErr: ErrInvalidCredentials},
		{name: "unknown user", password: stored, stored: stored, email: "unknown@example.com", status: StatusActive, expectedErr: ErrInvalidCredentials},
		{name: "disabled user", password: stored, stored: stored, status: StatusDisabled, expectedErr: ErrUserNotActive},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := &User{ID: 123, Email: "login@example.com", PasswordHash: tt.stored, Status: tt.status, Role: RoleUser}
			svc := newAuthService(&userRepoStub{user: user}, nil, nil, nil)
			email := tt.email
			if email == "" {
				email = user.Email
			}
			token, gotUser, err := svc.Login(context.Background(), email, tt.password)
			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				require.Empty(t, token)
				require.Nil(t, gotUser)
				return
			}
			require.NoError(t, err)
			require.Same(t, user, gotUser)
			claims, err := svc.ValidateToken(token)
			require.NoError(t, err)
			require.Equal(t, user.ID, claims.UserID)
			require.Equal(t, tt.stored, user.PasswordHash)
		})
	}
}

func TestAuthServiceValidatePasswordCredentialsRejectsStoredBcrypt(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("login-password"), bcrypt.MinCost)
	require.NoError(t, err)
	user := &User{ID: 123, Email: "login@example.com", PasswordHash: string(hash), Status: StatusActive}
	svc := newAuthService(&userRepoStub{user: user}, nil, nil, nil)

	gotUser, err := svc.ValidatePasswordCredentials(context.Background(), user.Email, user.PasswordHash)
	require.ErrorIs(t, err, ErrInvalidCredentials)
	require.Nil(t, gotUser)
	gotUser, err = svc.ValidatePasswordCredentials(context.Background(), user.Email, "login-password")
	require.NoError(t, err)
	require.Same(t, user, gotUser)
}
