package repository

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type modelRateLimitMaxPayload struct {
	until  time.Time
	reason string
}

func (m modelRateLimitMaxPayload) Match(value driver.Value) bool {
	raw, ok := value.([]byte)
	if !ok {
		return false
	}
	var payload map[string]string
	return json.Unmarshal(raw, &payload) == nil &&
		payload["rate_limit_reset_at"] == m.until.UTC().Format(time.RFC3339) &&
		payload["reason"] == m.reason
}

func TestAccountRepository_SetModelRateLimit_APIKey429MaxSQLContract(t *testing.T) {
	for _, reason := range []string{"openai_apikey_429", "model_not_found", ""} {
		t.Run(reason, func(t *testing.T) {
			var captured string
			matcher := sqlmock.QueryMatcherFunc(func(expected, actual string) error {
				if expected == "model_rate_limit_update" {
					captured = actual
					return nil
				}
				return sqlmock.QueryMatcherRegexp.Match(expected, actual)
			})
			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(matcher))
			require.NoError(t, err)
			client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
			t.Cleanup(func() { _ = client.Close() })
			repo := newAccountRepositoryWithSQL(client, db, nil)
			until := time.Now().Add(time.Minute)
			mock.ExpectExec("model_rate_limit_update").
				WithArgs("gpt-6.1-sol", modelRateLimitMaxPayload{until: until, reason: reason}, int64(748)).
				WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox")).
				WithArgs(service.SchedulerOutboxEventAccountChanged, int64(748), nil, nil, sqlmock.AnyArg()).
				WillReturnResult(sqlmock.NewResult(1, 1))
			require.NoError(t, repo.SetModelRateLimit(context.Background(), 748, "gpt-6.1-sol", until, reason))
			query := normalizeSQLWhitespace(captured)
			require.Contains(t, query, "WHEN $2::jsonb->>'reason' = 'openai_apikey_429'")
			require.Contains(t, query, "NULLIF(extra #>> ARRAY['model_rate_limits', $1, 'rate_limit_reset_at'], '')::timestamptz > ($2::jsonb->>'rate_limit_reset_at')::timestamptz")
			require.Contains(t, query, "THEN extra #> ARRAY['model_rate_limits', $1] ELSE $2::jsonb END")
			require.Contains(t, query, "WHERE id = $3 AND deleted_at IS NULL")
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
