package app

import (
	"testing"

	"github.com/morluto/gitcontribute/internal/radar"
)

func TestMCPCrossRepositoryEligibilityOrderMatchesRadar(t *testing.T) {
	t.Parallel()
	ordered := []radar.Eligibility{
		radar.EligibilityReadyToCode,
		radar.EligibilityNeedsDiagnosis,
		radar.EligibilityNeedsCoordination,
		radar.EligibilityBlocked,
	}
	for i, eligibility := range ordered {
		if got := eligibilityRank(string(eligibility)); got != i {
			t.Fatalf("eligibilityRank(%q) = %d, want %d", eligibility, got, i)
		}
	}
}
