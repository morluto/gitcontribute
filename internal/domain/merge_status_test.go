package domain

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseMergeStatusRejectsContradictions(t *testing.T) {
	t.Parallel()
	at := time.Unix(1, 0).UTC()
	for _, input := range []struct {
		known  bool
		merged bool
		at     time.Time
	}{
		{merged: true},
		{at: at},
		{known: true, at: at},
	} {
		if _, err := ParseMergeStatus(input.known, input.merged, input.at); err == nil {
			t.Fatalf("ParseMergeStatus(%v, %v, %v) succeeded", input.known, input.merged, input.at)
		}
	}
}

func TestMergeStatusJSONReparses(t *testing.T) {
	t.Parallel()
	want := MergedStatus(time.Unix(1, 0).UTC())
	payload, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got MergeStatus
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("merge status = %+v, want %+v", got, want)
	}
	if err := json.Unmarshal([]byte(`{"Known":false,"Merged":true,"MergedAt":"0001-01-01T00:00:00Z"}`), &got); err == nil {
		t.Fatal("contradictory merge status decoded")
	}
}
