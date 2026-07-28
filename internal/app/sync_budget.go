package app

import (
	"fmt"
)

const defaultSyncBatchMaxRequests = 1000

func syncRequestBudgetUnavailable(key string, required, remaining int) map[string]any {
	return map[string]any{
		"key": key, "status": "unavailable", "reason": "request_budget_exceeded",
		"message": fmt.Sprintf("planned sync requires %d requests but only %d remain", required, remaining),
	}
}
