package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
)

// Persisted limits survive restarts. Never trust caller-supplied forwarding
// headers: a reverse proxy must additionally enforce its per-client limits.
func (s *Store) AllowRegistration(agentID, remote, email string) (bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	now := s.clock().Unix()
	if _, err := tx.Exec(`DELETE FROM registration_limits WHERE window_start<=?`, now-3600); err != nil {
		return false, err
	}
	for _, limit := range []struct {
		key string
		max int
	}{{"ip:" + remote, 20}, {"email:" + strings.ToLower(email), 5}, {"site", 200}} {
		hash := sha256.Sum256([]byte(limit.key))
		key := hex.EncodeToString(hash[:])
		if _, err := tx.Exec(`INSERT INTO registration_limits(agent_id,bucket_key,window_start,attempts) VALUES (?,?,?,1) ON CONFLICT(agent_id,bucket_key) DO UPDATE SET attempts=attempts+1`, agentID, key, now); err != nil {
			return false, err
		}
		var count int
		if err := tx.QueryRow(`SELECT attempts FROM registration_limits WHERE agent_id=? AND bucket_key=?`, agentID, key).Scan(&count); err != nil {
			return false, err
		}
		if count > limit.max {
			return false, tx.Commit()
		}
	}
	return true, tx.Commit()
}

// The random marker is written before contacting the main site and stored in
// its admin-only notes. It proves creation provenance after a lost response.
// An email match or knowledge of another user's password alone cannot claim
// arbitrary main-site accounts for this agent.
func (s *Store) RegistrationMarker(agentID, email string) (string, error) {
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", err
	}
	marker := "agentapi-registration:" + hex.EncodeToString(nonce[:])
	_, err := s.db.Exec(`INSERT INTO registration_intents(agent_id,email,marker,created_at) VALUES (?,?,?,?) ON CONFLICT(agent_id,email) DO NOTHING`, agentID, strings.ToLower(email), marker, s.clock().Unix())
	if err != nil {
		return "", err
	}
	err = s.db.QueryRow(`SELECT marker FROM registration_intents WHERE agent_id=? AND email=?`, agentID, strings.ToLower(email)).Scan(&marker)
	return marker, err
}

func (s *Server) recoverRegistrationForTenant(ctx context.Context, agentID, userID, email, displayName string) (bool, error) {
	var marker string
	if err := s.store.db.QueryRow(`SELECT marker FROM registration_intents WHERE agent_id=? AND email=?`, agentID, strings.ToLower(email)).Scan(&marker); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	// This function is called only after successful main-site authentication.
	if s.cfg.MainAdminAPIKey == "" {
		return false, nil
	}
	headers := make(http.Header)
	headers.Set("x-api-key", s.cfg.MainAdminAPIKey)
	// Never interpolate an unchecked upstream identity into an admin path.
	for _, char := range userID {
		if char < '0' || char > '9' {
			return false, nil
		}
	}
	if userID == "" {
		return false, nil
	}
	resp, raw, err := s.main.request(ctx, http.MethodGet, s.main.endpoint("/admin/users/"+userID), nil, headers)
	if err != nil {
		return false, err
	}
	data, err := unwrapMainResponse(resp.StatusCode, raw)
	if err != nil {
		return false, err
	}
	var profile struct {
		Notes string `json:"notes"`
		Role  string `json:"role"`
		Email string `json:"email"`
	}
	if err := json.Unmarshal(data, &profile); err != nil {
		return false, err
	}
	if mainUserIDFromJSON(data) != userID || profile.Role != "user" || profile.Notes != marker || !strings.EqualFold(strings.TrimSpace(profile.Email), strings.TrimSpace(email)) {
		return false, nil
	}
	_, err = s.store.UpsertUser(agentID, userID, email, displayName)
	return err == nil, err
}

func registrationPeer(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
