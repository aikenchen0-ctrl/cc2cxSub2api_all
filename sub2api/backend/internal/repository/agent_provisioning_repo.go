package repository

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type agentProvisioningRepository struct {
	db        *sql.DB
	listenDSN string
}

func NewAgentProvisioningRepository(db *sql.DB) service.AgentProvisioningRepository {
	return &agentProvisioningRepository{db: db}
}

func ProvideAgentProvisioningRepository(db *sql.DB, cfg *config.Config) service.AgentProvisioningRepository {
	repo := &agentProvisioningRepository{db: db}
	if cfg != nil {
		repo.listenDSN = cfg.Database.DSN()
	}
	return repo
}

type agentProvisioningScanner interface {
	Scan(dest ...any) error
}

const agentProvisioningSelect = `
SELECT agent_id, slug, domain, display_name, owner_main_user_id, plan_id,
       brand_name, logo_url, status, current_step, recent_error,
       retryable, domain_status, created_by_user_id, request_id, created_at, updated_at
FROM agent_provisioning_agents`

func scanAgentProvisioningAgent(row agentProvisioningScanner) (*service.AgentProvisioningAgent, error) {
	var agent service.AgentProvisioningAgent
	var logoURL, recentError sql.NullString
	err := row.Scan(
		&agent.AgentID, &agent.Slug, &agent.Domain, &agent.DisplayName,
		&agent.OwnerMainUserID, &agent.PlanID, &agent.BrandName, &logoURL,
		&agent.Status, &agent.CurrentStep, &recentError, &agent.CanRetry,
		&agent.DomainStatus, &agent.CreatedByUserID, &agent.RequestID, &agent.CreatedAt, &agent.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if logoURL.Valid {
		agent.LogoURL = &logoURL.String
	}
	if recentError.Valid {
		agent.RecentError = recentError.String
	}
	return &agent, nil
}

func (r *agentProvisioningRepository) CreateAgent(ctx context.Context, actorUserID int64, keyHash, requestHash, requestID string, agent service.AgentProvisioningAgent) (*service.AgentProvisioningAgent, bool, error) {
	if r == nil || r.db == nil {
		return nil, false, errors.New("agent provisioning database is unavailable")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, fmt.Errorf("begin agent provisioning create: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, `
INSERT INTO agent_provisioning_idempotency (actor_user_id, key_hash, request_hash, operation, agent_id, request_id)
VALUES ($1, $2, $3, 'create', $4, $5)
ON CONFLICT (actor_user_id, key_hash) DO NOTHING`,
		actorUserID, keyHash, requestHash, agent.AgentID, requestID,
	)
	if err != nil {
		return nil, false, fmt.Errorf("reserve agent provisioning idempotency key: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return nil, false, fmt.Errorf("read idempotency reservation result: %w", err)
	}
	if inserted == 0 {
		var previousHash, previousOperation, previousAgentID string
		if err := tx.QueryRowContext(ctx, `
SELECT request_hash, operation, agent_id
FROM agent_provisioning_idempotency
WHERE actor_user_id = $1 AND key_hash = $2`, actorUserID, keyHash).
			Scan(&previousHash, &previousOperation, &previousAgentID); err != nil {
			return nil, false, fmt.Errorf("read existing agent provisioning idempotency key: %w", err)
		}
		if previousHash != requestHash || previousOperation != "create" {
			return nil, false, service.ErrAgentProvisioningIdempotency
		}
		agent, err := getAgentProvisioningAgent(ctx, tx, previousAgentID)
		if err != nil {
			return nil, false, err
		}
		if err := tx.Commit(); err != nil {
			return nil, false, fmt.Errorf("commit agent provisioning replay: %w", err)
		}
		return agent, true, nil
	}

	var ownerExists bool
	if err := tx.QueryRowContext(ctx, `
SELECT EXISTS (
    SELECT 1 FROM users
    WHERE id = $1 AND role = 'admin' AND status = 'active' AND deleted_at IS NULL
)`, agent.OwnerMainUserID).Scan(&ownerExists); err != nil {
		return nil, false, fmt.Errorf("validate agent owner: %w", err)
	}
	if !ownerExists {
		return nil, false, service.ErrAgentProvisioningOwnerNotFound
	}

	_, err = tx.ExecContext(ctx, `
INSERT INTO agent_provisioning_agents (
    agent_id, slug, domain, display_name, owner_main_user_id, plan_id,
    brand_name, logo_url, status, current_step, retryable, domain_status,
    created_by_user_id, request_id, created_at, updated_at
)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'pending', 'queued', FALSE, 'pending', $9, $10, NOW(), NOW())`,
		agent.AgentID, agent.Slug, agent.Domain, agent.DisplayName, agent.OwnerMainUserID,
		agent.PlanID, agent.BrandName, nullableLogoURL(agent.LogoURL), actorUserID, requestID,
	)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && string(pqErr.Code) == "23505" {
			if strings.Contains(pqErr.Constraint, "slug") || strings.Contains(pqErr.Constraint, "domain") {
				return nil, false, service.ErrAgentProvisioningSlugTaken
			}
		}
		return nil, false, fmt.Errorf("insert agent provisioning record: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO agent_provisioning_events (agent_id, actor_user_id, operation, from_status, to_status, request_id)
VALUES ($1, $2, 'create', NULL, 'pending', $3)`, agent.AgentID, actorUserID, requestID); err != nil {
		return nil, false, fmt.Errorf("record agent provisioning create event: %w", err)
	}
	created, err := getAgentProvisioningAgent(ctx, tx, agent.AgentID)
	if err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("commit agent provisioning create: %w", err)
	}
	return created, false, nil
}

func (r *agentProvisioningRepository) GetAgent(ctx context.Context, agentID string) (*service.AgentProvisioningAgent, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("agent provisioning database is unavailable")
	}
	agent, err := getAgentProvisioningAgent(ctx, r.db, agentID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrAgentProvisioningNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get agent provisioning record: %w", err)
	}
	return agent, nil
}

func (r *agentProvisioningRepository) ListAgents(ctx context.Context, filter service.AgentProvisioningListFilter) ([]service.AgentProvisioningAgent, int, error) {
	if r == nil || r.db == nil {
		return nil, 0, errors.New("agent provisioning database is unavailable")
	}
	search := ""
	if filter.Search != "" {
		search = "%" + filter.Search + "%"
	}
	var total int
	if err := r.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM agent_provisioning_agents
WHERE ($1 = '' OR status = $1)
  AND ($2 = '' OR slug ILIKE $2 OR domain ILIKE $2 OR display_name ILIKE $2 OR owner_main_user_id::text ILIKE $2)`,
		filter.Status, search).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count agent provisioning records: %w", err)
	}
	pageSize := filter.PageSize
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	page := filter.Page
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * pageSize
	rows, err := r.db.QueryContext(ctx, agentProvisioningSelect+`
WHERE ($1 = '' OR status = $1)
  AND ($2 = '' OR slug ILIKE $2 OR domain ILIKE $2 OR display_name ILIKE $2 OR owner_main_user_id::text ILIKE $2)
ORDER BY created_at DESC, agent_id ASC
LIMIT $3 OFFSET $4`, filter.Status, search, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list agent provisioning records: %w", err)
	}
	defer rows.Close()
	items := make([]service.AgentProvisioningAgent, 0, pageSize)
	for rows.Next() {
		item, err := scanAgentProvisioningAgent(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan agent provisioning record: %w", err)
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate agent provisioning records: %w", err)
	}
	return items, total, nil
}

// ListenAgentUpdates listens to PostgreSQL notifications emitted after an
// agent provisioning row commits. Consumers must read the current row through
// the management API/repository rather than treating the notification as a
// complete record.
func (r *agentProvisioningRepository) ListenAgentUpdates(ctx context.Context) (<-chan string, error) {
	if r == nil || strings.TrimSpace(r.listenDSN) == "" {
		return nil, errors.New("agent provisioning database listener is not configured")
	}
	listener := pq.NewListener(r.listenDSN, time.Second, 30*time.Second, nil)
	if err := listener.Listen("agent_provisioning_agents"); err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("listen for agent provisioning updates: %w", err)
	}
	updates := make(chan string)
	go func() {
		defer close(updates)
		defer listener.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case notification, ok := <-listener.Notify:
				if !ok {
					return
				}
				if notification == nil {
					continue
				}
				agentID := strings.TrimSpace(notification.Extra)
				if agentID == "" || len(agentID) > 64 {
					continue
				}
				select {
				case updates <- agentID:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return updates, nil
}

func (r *agentProvisioningRepository) ClaimAgent(ctx context.Context, agentID, tokenHash string, ttl time.Duration) (*service.AgentProvisioningAgent, time.Time, error) {
	if r == nil || r.db == nil {
		return nil, time.Time{}, errors.New("agent provisioning database is unavailable")
	}
	if ttl <= 0 || ttl > 10*time.Minute {
		return nil, time.Time{}, service.ErrAgentProvisioningInvalid
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("begin agent provisioning claim: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM agent_provisioning_agents WHERE agent_id=$1 FOR UPDATE`, agentID).Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, time.Time{}, service.ErrAgentProvisioningNotFound
		}
		return nil, time.Time{}, fmt.Errorf("lock agent provisioning claim target: %w", err)
	}
	if status != "pending" && status != "provisioning" {
		return nil, time.Time{}, service.ErrAgentProvisioningStateConflict
	}
	var expiresAt time.Time
	err = tx.QueryRowContext(ctx, `
INSERT INTO agent_provisioning_leases (agent_id, token_hash, expires_at, updated_at)
VALUES ($1, $2, NOW() + ($3 * INTERVAL '1 second'), NOW())
ON CONFLICT (agent_id) DO UPDATE
SET token_hash=EXCLUDED.token_hash, expires_at=EXCLUDED.expires_at, updated_at=NOW()
WHERE agent_provisioning_leases.expires_at <= NOW()
RETURNING expires_at`, agentID, tokenHash, int64(ttl.Seconds())).Scan(&expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, time.Time{}, service.ErrAgentProvisioningLeaseHeld
	}
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("claim agent provisioning lease: %w", err)
	}
	agent, err := getAgentProvisioningAgent(ctx, tx, agentID)
	if err != nil {
		return nil, time.Time{}, err
	}
	if err := tx.Commit(); err != nil {
		return nil, time.Time{}, fmt.Errorf("commit agent provisioning claim: %w", err)
	}
	return agent, expiresAt.UTC(), nil
}

func (r *agentProvisioningRepository) RenewAgentLease(ctx context.Context, agentID, tokenHash string, ttl time.Duration) (*service.AgentProvisioningAgent, time.Time, error) {
	if r == nil || r.db == nil {
		return nil, time.Time{}, errors.New("agent provisioning database is unavailable")
	}
	if ttl <= 0 || ttl > 10*time.Minute {
		return nil, time.Time{}, service.ErrAgentProvisioningInvalid
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("begin agent provisioning lease renewal: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM agent_provisioning_agents WHERE agent_id=$1 FOR UPDATE`, agentID).Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, time.Time{}, service.ErrAgentProvisioningNotFound
		}
		return nil, time.Time{}, fmt.Errorf("lock agent provisioning lease target: %w", err)
	}
	if status != "pending" && status != "provisioning" {
		return nil, time.Time{}, service.ErrAgentProvisioningLeaseLost
	}
	var expiresAt time.Time
	err = tx.QueryRowContext(ctx, `
UPDATE agent_provisioning_leases
SET expires_at=NOW() + ($3 * INTERVAL '1 second'), updated_at=NOW()
WHERE agent_id=$1 AND token_hash=$2 AND expires_at > NOW()
RETURNING expires_at`, agentID, tokenHash, int64(ttl.Seconds())).Scan(&expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, time.Time{}, service.ErrAgentProvisioningLeaseLost
	}
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("renew agent provisioning lease: %w", err)
	}
	agent, err := getAgentProvisioningAgent(ctx, tx, agentID)
	if err != nil {
		return nil, time.Time{}, err
	}
	if err := tx.Commit(); err != nil {
		return nil, time.Time{}, fmt.Errorf("commit agent provisioning lease renewal: %w", err)
	}
	return agent, expiresAt.UTC(), nil
}

func (r *agentProvisioningRepository) TransitionAgent(ctx context.Context, actorUserID int64, keyHash, requestHash, operation, agentID, requestID, leaseTokenHash string) (*service.AgentProvisioningAgent, bool, error) {
	if r == nil || r.db == nil {
		return nil, false, errors.New("agent provisioning database is unavailable")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, fmt.Errorf("begin agent provisioning transition: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, `
INSERT INTO agent_provisioning_idempotency (actor_user_id, key_hash, request_hash, operation, agent_id, request_id)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (actor_user_id, key_hash) DO NOTHING`,
		actorUserID, keyHash, requestHash, operation, agentID, requestID,
	)
	if err != nil {
		return nil, false, fmt.Errorf("reserve agent provisioning action key: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return nil, false, fmt.Errorf("read action idempotency result: %w", err)
	}
	if inserted == 0 {
		var previousHash, previousOperation, previousAgentID string
		if err := tx.QueryRowContext(ctx, `
SELECT request_hash, operation, agent_id
FROM agent_provisioning_idempotency
WHERE actor_user_id = $1 AND key_hash = $2`, actorUserID, keyHash).
			Scan(&previousHash, &previousOperation, &previousAgentID); err != nil {
			return nil, false, fmt.Errorf("read existing action idempotency key: %w", err)
		}
		if previousHash != requestHash || previousOperation != operation || previousAgentID != agentID {
			return nil, false, service.ErrAgentProvisioningIdempotency
		}
		agent, err := getAgentProvisioningAgent(ctx, tx, agentID)
		if err != nil {
			return nil, false, err
		}
		if err := tx.Commit(); err != nil {
			return nil, false, fmt.Errorf("commit agent provisioning action replay: %w", err)
		}
		return agent, true, nil
	}

	var currentStatus, currentStep string
	var retryable bool
	if err := tx.QueryRowContext(ctx, `SELECT status, current_step, retryable FROM agent_provisioning_agents WHERE agent_id = $1 FOR UPDATE`, agentID).Scan(&currentStatus, &currentStep, &retryable); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, service.ErrAgentProvisioningNotFound
		}
		return nil, false, fmt.Errorf("lock agent provisioning record: %w", err)
	}
	targetStatus, step, allowed := agentProvisioningTargetState(operation, currentStatus, currentStep, retryable)
	if !allowed {
		return nil, false, service.ErrAgentProvisioningStateConflict
	}
	if targetStatus != currentStatus {
		if operation == "activate" && leaseTokenHash != "" {
			if err := lockValidAgentProvisioningLease(ctx, tx, agentID, leaseTokenHash); err != nil {
				return nil, false, err
			}
		}
		var updateErr error
		switch operation {
		case "suspend":
			_, updateErr = tx.ExecContext(ctx, `
UPDATE agent_provisioning_agents
SET status = $1, status_before_suspend = $2, current_step = $3, updated_at = NOW()
WHERE agent_id = $4`, targetStatus, currentStatus, step, agentID)
		case "resume":
			_, updateErr = tx.ExecContext(ctx, `
UPDATE agent_provisioning_agents
SET status = $1, status_before_suspend = NULL, current_step = $2, updated_at = NOW()
WHERE agent_id = $3`, targetStatus, step, agentID)
		case "retry":
			_, updateErr = tx.ExecContext(ctx, `
UPDATE agent_provisioning_agents
SET status = $1, current_step = $2, recent_error = NULL, retryable = FALSE, domain_status = $3, updated_at = NOW()
WHERE agent_id = $4`, targetStatus, step, agentProvisioningDomainStatusForRetry(step), agentID)
		case "activate":
			_, updateErr = tx.ExecContext(ctx, `
UPDATE agent_provisioning_agents
SET status = $1, status_before_suspend = NULL, current_step = $2, retryable = FALSE, domain_status = 'ready', updated_at = NOW()
WHERE agent_id = $3`, targetStatus, step, agentID)
		case "revoke":
			_, updateErr = tx.ExecContext(ctx, `
UPDATE agent_provisioning_agents
SET status = $1, current_step = $2, retryable = FALSE, revoked_at = NOW(), updated_at = NOW()
WHERE agent_id = $3`, targetStatus, step, agentID)
		default:
			return nil, false, service.ErrAgentProvisioningInvalid
		}
		if updateErr != nil {
			return nil, false, fmt.Errorf("update agent provisioning state: %w", updateErr)
		}
		if operation == "revoke" {
			if _, err := tx.ExecContext(ctx, `
UPDATE agent_runtime_credentials SET status='revoked', revoked_at=NOW()
WHERE agent_id=$1 AND status='active'`, agentID); err != nil {
				return nil, false, fmt.Errorf("revoke runtime credentials with Agent: %w", err)
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM agent_provisioning_leases WHERE agent_id=$1`, agentID); err != nil {
			return nil, false, fmt.Errorf("release agent provisioning lease after transition: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO agent_provisioning_events (agent_id, actor_user_id, operation, from_status, to_status, request_id)
VALUES ($1, $2, $3, $4, $5, $6)`, agentID, actorUserID, operation, currentStatus, targetStatus, requestID); err != nil {
			return nil, false, fmt.Errorf("record agent provisioning transition: %w", err)
		}
	}
	agent, err := getAgentProvisioningAgent(ctx, tx, agentID)
	if err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("commit agent provisioning transition: %w", err)
	}
	return agent, false, nil
}

func (r *agentProvisioningRepository) ReportProgress(ctx context.Context, actorUserID int64, keyHash, requestHash, requestID string, progress service.AgentProvisioningProgress, leaseTokenHash string) (*service.AgentProvisioningAgent, bool, error) {
	if r == nil || r.db == nil {
		return nil, false, errors.New("agent provisioning database is unavailable")
	}
	const operation = "progress"
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, fmt.Errorf("begin agent provisioning progress update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var currentStatus, currentStep, currentDomainStatus string
	if err := tx.QueryRowContext(ctx, `
SELECT status, current_step, domain_status
FROM agent_provisioning_agents
WHERE agent_id = $1
FOR UPDATE`, progress.AgentID).Scan(&currentStatus, &currentStep, &currentDomainStatus); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, service.ErrAgentProvisioningNotFound
		}
		return nil, false, fmt.Errorf("lock agent provisioning progress record: %w", err)
	}
	if err := lockValidAgentProvisioningLease(ctx, tx, progress.AgentID, leaseTokenHash); err != nil {
		return nil, false, err
	}

	result, err := tx.ExecContext(ctx, `
INSERT INTO agent_provisioning_idempotency (actor_user_id, key_hash, request_hash, operation, agent_id, request_id)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (actor_user_id, key_hash) DO NOTHING`,
		actorUserID, keyHash, requestHash, operation, progress.AgentID, requestID,
	)
	if err != nil {
		return nil, false, fmt.Errorf("reserve agent provisioning progress key: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return nil, false, fmt.Errorf("read progress idempotency result: %w", err)
	}
	if inserted == 0 {
		var previousHash, previousOperation, previousAgentID string
		if err := tx.QueryRowContext(ctx, `
SELECT request_hash, operation, agent_id
FROM agent_provisioning_idempotency
WHERE actor_user_id = $1 AND key_hash = $2`, actorUserID, keyHash).
			Scan(&previousHash, &previousOperation, &previousAgentID); err != nil {
			return nil, false, fmt.Errorf("read existing progress idempotency key: %w", err)
		}
		if previousHash != requestHash || previousOperation != operation || previousAgentID != progress.AgentID {
			return nil, false, service.ErrAgentProvisioningIdempotency
		}
		agent, err := getAgentProvisioningAgent(ctx, tx, progress.AgentID)
		if err != nil {
			return nil, false, err
		}
		if err := tx.Commit(); err != nil {
			return nil, false, fmt.Errorf("commit agent provisioning progress replay: %w", err)
		}
		return agent, true, nil
	}

	targetStatus, targetStep, targetDomainStatus, recentError, retryable, allowed := agentProvisioningProgressTarget(
		progress, currentStatus, currentStep, currentDomainStatus,
	)
	if !allowed {
		return nil, false, service.ErrAgentProvisioningStateConflict
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE agent_provisioning_agents
SET status = $1, current_step = $2, domain_status = $3, recent_error = NULLIF($4, ''), retryable = $5, updated_at = NOW()
WHERE agent_id = $6`, targetStatus, targetStep, targetDomainStatus, recentError, retryable, progress.AgentID); err != nil {
		return nil, false, fmt.Errorf("update agent provisioning progress: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO agent_provisioning_events (agent_id, actor_user_id, operation, from_status, to_status, request_id)
VALUES ($1, $2, $3, $4, $5, $6)`, progress.AgentID, actorUserID, operation, currentStatus, targetStatus, requestID); err != nil {
		return nil, false, fmt.Errorf("record agent provisioning progress: %w", err)
	}
	if targetStatus == "failed" {
		if _, err := tx.ExecContext(ctx, `DELETE FROM agent_provisioning_leases WHERE agent_id=$1 AND token_hash=$2`, progress.AgentID, leaseTokenHash); err != nil {
			return nil, false, fmt.Errorf("release failed agent provisioning lease: %w", err)
		}
	} else if _, err := tx.ExecContext(ctx, `
UPDATE agent_provisioning_leases SET expires_at=NOW() + (90 * INTERVAL '1 second'), updated_at=NOW()
WHERE agent_id=$1 AND token_hash=$2 AND expires_at > NOW()`, progress.AgentID, leaseTokenHash); err != nil {
		return nil, false, fmt.Errorf("renew agent provisioning lease after progress: %w", err)
	}
	agent, err := getAgentProvisioningAgent(ctx, tx, progress.AgentID)
	if err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("commit agent provisioning progress: %w", err)
	}
	return agent, false, nil
}

func lockValidAgentProvisioningLease(ctx context.Context, tx *sql.Tx, agentID, tokenHash string) error {
	var storedTokenHash string
	err := tx.QueryRowContext(ctx, `
SELECT token_hash FROM agent_provisioning_leases
WHERE agent_id=$1 AND token_hash=$2 AND expires_at > NOW()
FOR UPDATE`, agentID, tokenHash).Scan(&storedTokenHash)
	if errors.Is(err, sql.ErrNoRows) {
		return service.ErrAgentProvisioningLeaseLost
	}
	if err != nil {
		return fmt.Errorf("validate agent provisioning lease: %w", err)
	}
	if storedTokenHash != tokenHash {
		return service.ErrAgentProvisioningLeaseLost
	}
	return nil
}

func (r *agentProvisioningRepository) StoreRuntimeCredentialHashes(ctx context.Context, agentID, controlHash, modelHash, leaseTokenHash string) error {
	if r == nil || r.db == nil {
		return errors.New("agent provisioning database is unavailable")
	}
	if !validRuntimeCredentialHash(controlHash) || !validRuntimeCredentialHash(modelHash) || controlHash == modelHash || !validRuntimeCredentialHash(leaseTokenHash) {
		return service.ErrAgentProvisioningInvalid
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin runtime credential registration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM agent_provisioning_agents WHERE agent_id=$1 FOR UPDATE`, agentID).Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return service.ErrAgentProvisioningNotFound
		}
		return fmt.Errorf("lock agent for runtime credential registration: %w", err)
	}
	if status != "pending" && status != "provisioning" {
		return service.ErrAgentProvisioningStateConflict
	}
	if err := lockValidAgentProvisioningLease(ctx, tx, agentID, leaseTokenHash); err != nil {
		return err
	}
	// The worker may have committed registration and then lost the response.
	// In that case retrying the same pair must be safe. Never reactivate hashes
	// that were revoked, though: token_hash is intentionally unique forever so
	// a stale Secret can never become valid again by replaying this call.
	existingRows, err := tx.QueryContext(ctx, `
SELECT agent_id, purpose, token_hash, status
FROM agent_runtime_credentials
WHERE token_hash IN ($1, $2)
FOR UPDATE`, controlHash, modelHash)
	if err != nil {
		return fmt.Errorf("check existing runtime credential hashes: %w", err)
	}
	type storedRuntimeCredential struct {
		agentID string
		purpose string
		hash    string
		status  string
	}
	existing := make([]storedRuntimeCredential, 0, 2)
	for existingRows.Next() {
		var credential storedRuntimeCredential
		if err := existingRows.Scan(&credential.agentID, &credential.purpose, &credential.hash, &credential.status); err != nil {
			_ = existingRows.Close()
			return fmt.Errorf("scan existing runtime credential hash: %w", err)
		}
		existing = append(existing, credential)
	}
	if err := existingRows.Err(); err != nil {
		_ = existingRows.Close()
		return fmt.Errorf("iterate existing runtime credential hashes: %w", err)
	}
	if err := existingRows.Close(); err != nil {
		return fmt.Errorf("close existing runtime credential hashes: %w", err)
	}
	for _, credential := range existing {
		wantPurpose := "control"
		if credential.hash == modelHash {
			wantPurpose = "model"
		}
		if credential.agentID != agentID || credential.purpose != wantPurpose || credential.status != "active" {
			return service.ErrAgentRuntimeCredentialInvalid
		}
	}
	if len(existing) == 2 {
		if _, err := tx.ExecContext(ctx, `
UPDATE agent_provisioning_leases SET expires_at=NOW() + (90 * INTERVAL '1 second'), updated_at=NOW()
WHERE agent_id=$1 AND token_hash=$2 AND expires_at > NOW()`, agentID, leaseTokenHash); err != nil {
			return fmt.Errorf("renew lease after runtime credential registration replay: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit runtime credential registration replay: %w", err)
		}
		return nil
	}
	if len(existing) != 0 {
		return service.ErrAgentRuntimeCredentialInvalid
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE agent_runtime_credentials
SET status='revoked', revoked_at=NOW()
WHERE agent_id=$1 AND status='active'`, agentID); err != nil {
		return fmt.Errorf("rotate previous runtime credentials: %w", err)
	}
	for _, credential := range []struct{ purpose, hash string }{{"control", controlHash}, {"model", modelHash}} {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO agent_runtime_credentials (agent_id, purpose, token_hash, status)
VALUES ($1, $2, $3, 'active')`, agentID, credential.purpose, credential.hash); err != nil {
			return fmt.Errorf("store runtime credential hash: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE agent_provisioning_leases SET expires_at=NOW() + (90 * INTERVAL '1 second'), updated_at=NOW()
WHERE agent_id=$1 AND token_hash=$2 AND expires_at > NOW()`, agentID, leaseTokenHash); err != nil {
		return fmt.Errorf("renew lease after runtime credential registration: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit runtime credential registration: %w", err)
	}
	return nil
}

func (r *agentProvisioningRepository) ResolveRuntimeCredential(ctx context.Context, tokenHash string) (*service.AgentRuntimeCredential, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("agent provisioning database is unavailable")
	}
	if !validRuntimeCredentialHash(tokenHash) {
		return nil, service.ErrAgentRuntimeCredentialInvalid
	}
	var credential service.AgentRuntimeCredential
	var rawModelAllowlist string
	err := r.db.QueryRowContext(ctx, `
SELECT c.agent_id, a.owner_main_user_id, c.purpose, a.status, COALESCE(c.model_allowlist::text, '')
FROM agent_runtime_credentials c
JOIN agent_provisioning_agents a ON a.agent_id=c.agent_id
WHERE c.token_hash=$1 AND c.status='active'`, tokenHash).Scan(
		&credential.AgentID, &credential.OwnerMainUserID, &credential.Purpose, &credential.AgentStatus, &rawModelAllowlist,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("resolve agent runtime credential: %w", err)
	}
	if rawModelAllowlist != "" {
		if err := json.Unmarshal([]byte(rawModelAllowlist), &credential.ModelAllowlist); err != nil {
			return nil, fmt.Errorf("decode Agent runtime model allowlist: %w", err)
		}
		credential.ModelAllowlistWasSet = true
	}
	// Keep last_used_at useful for rotation/audit without turning every model
	// request into a write; updates are throttled to one per token per minute.
	if _, err := r.db.ExecContext(ctx, `
UPDATE agent_runtime_credentials SET last_used_at=NOW()
WHERE token_hash=$1 AND status='active'
  AND (last_used_at IS NULL OR last_used_at < NOW() - INTERVAL '1 minute')`, tokenHash); err != nil {
		return nil, fmt.Errorf("touch agent runtime credential: %w", err)
	}
	return &credential, nil
}

func (r *agentProvisioningRepository) SetRuntimeModelAllowlist(ctx context.Context, agentID string, models []string) error {
	if r == nil || r.db == nil {
		return errors.New("agent provisioning database is unavailable")
	}
	encoded, err := json.Marshal(models)
	if err != nil {
		return fmt.Errorf("encode Agent runtime model allowlist: %w", err)
	}
	result, err := r.db.ExecContext(ctx, `
UPDATE agent_runtime_credentials
SET model_allowlist=$2::jsonb
WHERE agent_id=$1 AND purpose='model' AND status='active'`, agentID, string(encoded))
	if err != nil {
		return fmt.Errorf("update Agent runtime model allowlist: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read Agent runtime model allowlist update count: %w", err)
	}
	if changed == 0 {
		return service.ErrAgentRuntimeCredentialInvalid
	}
	return nil
}

func (r *agentProvisioningRepository) RevokeRuntimeCredentials(ctx context.Context, agentID string) error {
	if r == nil || r.db == nil {
		return errors.New("agent provisioning database is unavailable")
	}
	var exists bool
	if err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM agent_provisioning_agents WHERE agent_id=$1)`, agentID).Scan(&exists); err != nil {
		return fmt.Errorf("check agent before runtime credential revocation: %w", err)
	}
	if !exists {
		return service.ErrAgentProvisioningNotFound
	}
	if _, err := r.db.ExecContext(ctx, `
UPDATE agent_runtime_credentials SET status='revoked', revoked_at=NOW()
WHERE agent_id=$1 AND status='active'`, agentID); err != nil {
		return fmt.Errorf("revoke agent runtime credentials: %w", err)
	}
	return nil
}

func (r *agentProvisioningRepository) MapRuntimeUser(ctx context.Context, agentID string, userID int64) error {
	if r == nil || r.db == nil {
		return errors.New("agent provisioning database is unavailable")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin agent runtime user mapping: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM agent_provisioning_agents WHERE agent_id=$1 FOR UPDATE`, agentID).Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return service.ErrAgentProvisioningNotFound
		}
		return fmt.Errorf("load agent for runtime user mapping: %w", err)
	}
	if status != "active" {
		return service.ErrAgentRuntimeAgentInactive
	}
	result, err := tx.ExecContext(ctx, `
INSERT INTO agent_runtime_user_mappings (agent_id, main_user_id)
VALUES ($1, $2)
ON CONFLICT (agent_id, main_user_id) DO NOTHING`, agentID, userID)
	if err != nil {
		var pgErr *pq.Error
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return service.ErrAgentRuntimeUserScopeConflict
		}
		return fmt.Errorf("insert runtime user mapping: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read runtime user mapping result: %w", err)
	}
	if inserted == 0 {
		var mappedAgentID string
		if err := tx.QueryRowContext(ctx, `SELECT agent_id FROM agent_runtime_user_mappings WHERE main_user_id=$1`, userID).Scan(&mappedAgentID); err != nil {
			return fmt.Errorf("read existing runtime user mapping: %w", err)
		}
		if mappedAgentID != agentID {
			return service.ErrAgentRuntimeUserScopeConflict
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit agent runtime user mapping: %w", err)
	}
	return nil
}

func (r *agentProvisioningRepository) IsRuntimeUser(ctx context.Context, agentID string, userID int64) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("agent provisioning database is unavailable")
	}
	var mapped bool
	err := r.db.QueryRowContext(ctx, `
SELECT EXISTS(SELECT 1 FROM agent_runtime_user_mappings WHERE agent_id=$1 AND main_user_id=$2)`, agentID, userID).Scan(&mapped)
	if err != nil {
		return false, fmt.Errorf("check runtime user mapping: %w", err)
	}
	return mapped, nil
}

func (r *agentProvisioningRepository) IsActiveRuntimeUser(ctx context.Context, agentID string, userID int64) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("agent provisioning database is unavailable")
	}
	var active bool
	err := r.db.QueryRowContext(ctx, `
SELECT EXISTS (
    SELECT 1
    FROM agent_runtime_user_mappings AS mappings
    JOIN users ON users.id = mappings.main_user_id
    WHERE mappings.agent_id = $1
      AND mappings.main_user_id = $2
      AND users.status = 'active'
      AND users.deleted_at IS NULL
)`, agentID, userID).Scan(&active)
	if err != nil {
		return false, fmt.Errorf("check active Agent runtime user mapping: %w", err)
	}
	return active, nil
}

func validRuntimeCredentialHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}

func agentProvisioningProgressTarget(progress service.AgentProvisioningProgress, currentStatus, currentStep, currentDomainStatus string) (status, step, domainStatus, recentError string, retryable, allowed bool) {
	if currentStatus != "pending" && currentStatus != "provisioning" && currentStatus != "failed" {
		return "", "", "", "", false, false
	}
	if progress.Step == "failed" {
		message, known := service.AgentProvisioningFailureMessage(progress.FailureCode)
		if !known || currentStatus != "provisioning" {
			return "", "", "", "", false, false
		}
		failureStep := progress.FailureStep
		if failureStep == "" {
			// Older workers did not include the checkpoint explicitly. The
			// current authoritative step is the only safe compatibility value.
			failureStep = currentStep
		}
		validFailureStep := false
		for _, candidate := range service.AgentProvisioningSteps() {
			if candidate == currentStep && candidate == failureStep {
				validFailureStep = true
				break
			}
		}
		if !validFailureStep {
			return "", "", "", "", false, false
		}
		domainStatus = currentDomainStatus
		if progress.FailureCode == "dns_failed" || progress.FailureCode == "tls_failed" {
			domainStatus = "failed"
		}
		return "failed", failureStep, domainStatus, message, progress.Retryable, true
	}

	if currentStatus == "pending" && currentStep == "queued" {
		if progress.Step != "validating" {
			return "", "", "", "", false, false
		}
	} else {
		steps := service.AgentProvisioningSteps()
		currentIndex := -1
		for index, value := range steps {
			if value == currentStep {
				currentIndex = index
				break
			}
		}
		if currentIndex < 0 || currentIndex+1 >= len(steps) || steps[currentIndex+1] != progress.Step {
			return "", "", "", "", false, false
		}
	}

	domainStatus = currentDomainStatus
	switch progress.Step {
	case "validating", "bundle_generated":
		domainStatus = "pending"
	case "deploying", "domain_check":
		domainStatus = "configuring"
	case "tls_check":
		domainStatus = "configuring"
	case "readiness_check":
		if currentDomainStatus != "configuring" {
			return "", "", "", "", false, false
		}
		domainStatus = "ready"
	default:
		return "", "", "", "", false, false
	}
	return "provisioning", progress.Step, domainStatus, "", false, true
}

func agentProvisioningTargetState(operation, current, currentStep string, canRetry bool) (target, step string, allowed bool) {
	switch operation {
	case "activate":
		if current == "pending" || current == "provisioning" || current == "failed" {
			return "active", "ready", true
		}
		if current == "active" {
			return current, "ready", true
		}
	case "suspend":
		if current == "active" {
			return "suspended", "suspend_requested", true
		}
		if current == "suspended" {
			return current, "suspend_requested", true
		}
	case "resume":
		if current == "suspended" {
			return "active", "resume_requested", true
		}
		if current == "active" {
			return current, "resume_requested", true
		}
	case "retry":
		if current == "failed" && canRetry {
			for _, candidate := range service.AgentProvisioningSteps() {
				if candidate == currentStep {
					return "pending", currentStep, true
				}
			}
			// Pre-checkpoint records stored the literal "failed" step. Restart
			// those safely from the first worker step.
			return "pending", "queued", true
		}
	case "revoke":
		if current != "revoked" {
			return "revoked", "revoke_requested", true
		}
		return current, "revoke_requested", true
	}
	return "", "", false
}

func agentProvisioningDomainStatusForRetry(step string) string {
	switch step {
	case "tls_check":
		// DNS already passed before TLS was attempted, so preserve the
		// configured state expected by the next readiness checkpoint.
		return "configuring"
	case "readiness_check":
		// Both DNS and TLS already passed before readiness was attempted.
		return "ready"
	default:
		return "pending"
	}
}

func getAgentProvisioningAgent(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, agentID string) (*service.AgentProvisioningAgent, error) {
	query := agentProvisioningSelect + ` WHERE agent_id = $1`
	return scanAgentProvisioningAgent(queryer.QueryRowContext(ctx, query, agentID))
}

func nullableLogoURL(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}
