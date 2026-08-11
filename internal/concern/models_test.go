package concern

import (
	"encoding/json"
	"testing"
	"time"
)

func TestPromotionJSONRoundTripPreservesSealedVariant(t *testing.T) {
	t.Parallel()
	at := time.Unix(100, 0).UTC()
	promotion, err := NewOpportunityPromotion(" inv-1 ", " hyp-1 ", " opp-1 ", at)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(promotion)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Promotion
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Kind() != "opportunity" || decoded.InvestigationID() != "inv-1" || decoded.HypothesisID() != "hyp-1" || decoded.OpportunityID() != "opp-1" || !decoded.PromotedAt().Equal(at) {
		t.Fatalf("decoded promotion = %+v", decoded)
	}
}

func TestPromotionJSONRejectsMixedIdentity(t *testing.T) {
	t.Parallel()
	var promotion Promotion
	if err := json.Unmarshal([]byte(`{"Kind":"investigation","InvestigationID":"inv-1","HypothesisID":"hyp-1","OpportunityID":"opp-1"}`), &promotion); err == nil {
		t.Fatal("expected investigation promotion with an opportunity identity to be rejected")
	}
	if status, err := ParseStatus(" accepted "); err != nil || status != StatusAccepted {
		t.Fatalf("parsed status = %q, %v", status, err)
	}
	if kind, err := ParseLinkKind(" hotspot "); err != nil || kind != LinkHotspot {
		t.Fatalf("parsed link kind = %q, %v", kind, err)
	}
}
