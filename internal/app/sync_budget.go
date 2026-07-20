package app

import (
	"fmt"
)

const defaultSyncBatchMaxRequests = 1000

func normalizeSyncBatchMaxRequests(value int) (int, error) {
	if value == 0 {
		return defaultSyncBatchMaxRequests, nil
	}
	if value < syncFixedRequestCost() || value > defaultSyncBatchMaxRequests {
		return 0, fmt.Errorf("max requests must be between %d and %d", syncFixedRequestCost(), defaultSyncBatchMaxRequests)
	}
	return value, nil
}

func syncRequestBudgetUnavailable(key string, required, remaining int) map[string]any {
	return map[string]any{
		"key": key, "status": "unavailable", "reason": "request_budget_exceeded",
		"message": fmt.Sprintf("planned sync requires %d requests but only %d remain", required, remaining),
	}
}
