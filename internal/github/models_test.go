package github

import (
	"encoding/json"
	"testing"
)

func TestPullRequestMergeStateJSONRejectsContradictoryKnownFlag(t *testing.T) {
	t.Parallel()
	for _, data := range []string{
		`{"MergeStateStatus":"CLEAN","Mergeable":"MERGEABLE","MergeableKnown":false}`,
		`{"MergeStateStatus":"UNKNOWN","Mergeable":"UNKNOWN","MergeableKnown":true}`,
	} {
		var state PullRequestMergeState
		if err := json.Unmarshal([]byte(data), &state); err == nil {
			t.Fatalf("Unmarshal accepted contradictory mergeability: %s", data)
		}
	}
}

func TestPullRequestMergeStateJSONRoundTrip(t *testing.T) {
	t.Parallel()
	mergeable := "MERGEABLE"
	want := NewPullRequestMergeState("CLEAN", &mergeable)
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got PullRequestMergeState
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	value, known := got.Mergeability()
	if got.MergeStateStatus != "CLEAN" || !known || value != mergeable {
		t.Fatalf("round trip = %+v, mergeability = %q, known = %v", got, value, known)
	}
}
