package admin

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpdateGroupRequestLimitFieldsTriState(t *testing.T) {
	var req UpdateGroupRequest
	require.NoError(t, json.Unmarshal([]byte(`{"daily_limit_usd":null}`), &req))
	limit := req.DailyLimitUSD.ToServiceInput()
	require.NotNil(t, limit)
	require.Negative(t, *limit)
}
