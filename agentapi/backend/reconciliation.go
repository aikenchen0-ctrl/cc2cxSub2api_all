package main

// Move each attempted lookup to the back of this agent's pending queue. A
// monotonic sequence (rather than second-resolution timestamps) ensures fair
// rotation even when multiple batches run within the same second.
func (s *Store) MarkReconciliationAttempt(agentID, requestID string) error {
	_, err := s.db.Exec(`UPDATE settlements SET reconcile_sequence=(SELECT COALESCE(MAX(reconcile_sequence),0)+1 FROM settlements WHERE agent_id=?) WHERE agent_id=? AND request_id=? AND status='pending'`, agentID, agentID, requestID)
	return err
}
