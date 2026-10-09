package main

import (
	"strconv"
	"time"
)

// Persist the cancellation and its rate-limit event in one transaction. Admin
// cancellations use the admin path and do not count against a member's quota.
func (s *Store) cancelStationUserOrder(agentID, userID string, id int64, requestID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := s.clock().UTC().Unix()
	result, err := tx.Exec(`UPDATE station_payment_orders SET status='CANCELLED', updated_at=? WHERE agent_id=? AND main_user_id=? AND id=? AND status='PENDING'`, now, agentID, userID, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errNotFound
	}
	_, err = tx.Exec(`INSERT INTO audit_events(actor_type, actor_id, agent_id, operation, target_type, target_id, request_id, result, reason, created_at) VALUES ('member', ?, ?, 'billing.order.cancel', 'station_order', ?, ?, 'success', '', ?)`, userID, agentID, strconv.FormatInt(id, 10), requestID, now)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func stationCancelWindowStart(now time.Time, settings StationPaymentSettings) time.Time {
	window := settings.CancelRateLimitWindow
	if window < 1 {
		window = 1
	}
	unit := time.Hour
	switch settings.CancelRateLimitUnit {
	case "minute":
		unit = time.Minute
	case "day", "":
		unit = 24 * time.Hour
	}
	if settings.CancelRateLimitMode == "fixed" {
		if unit == 24*time.Hour {
			y, m, d := now.Date()
			return time.Date(y, m, d, 0, 0, 0, 0, now.Location()).AddDate(0, 0, -(window - 1))
		}
		return now.Truncate(unit).Add(-time.Duration(window-1) * unit)
	}
	return now.Add(-time.Duration(window) * unit)
}

func (s *Store) stationCancelRateLimited(agentID, userID string, settings StationPaymentSettings) (bool, error) {
	if !settings.CancelRateLimitEnabled {
		return false, nil
	}
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM audit_events WHERE agent_id=? AND actor_id=? AND actor_type='member' AND operation='billing.order.cancel' AND result='success' AND created_at>=?`, agentID, userID, stationCancelWindowStart(s.clock(), settings).Unix()).Scan(&count)
	return count >= settings.CancelRateLimitMax, err
}
