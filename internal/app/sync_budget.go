package app

import (
	"fmt"
)

const defaultSyncBatchMaxRequests = 1000

func syncRequestBudgetMessage(required, remaining int) string {
	return fmt.Sprintf("planned sync requires %d requests but only %d remain", required, remaining)
}
