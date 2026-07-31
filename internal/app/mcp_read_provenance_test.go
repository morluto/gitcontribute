package app

import (
	"strings"
	"testing"
)

func TestOfflineReadProvenanceBindsQueryAndWatermark(t *testing.T) {
	t.Parallel()
	input := struct {
		Query string `json:"query"`
	}{Query: "immutable artifacts"}

	first, err := offlineReadProvenance("search", 7, input, true, false, false)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := offlineReadProvenance("search", 7, input, true, false, false)
	if err != nil {
		t.Fatal(err)
	}
	newer, err := offlineReadProvenance("search", 8, input, true, false, false)
	if err != nil {
		t.Fatal(err)
	}
	differentQuery, err := offlineReadProvenance("search", 7, struct {
		Query string `json:"query"`
	}{Query: "coverage"}, true, false, false)
	if err != nil {
		t.Fatal(err)
	}

	if first.SnapshotToken != repeated.SnapshotToken || first.QueryDigestSHA256 != repeated.QueryDigestSHA256 {
		t.Fatalf("same read identity changed: first=%+v repeated=%+v", first, repeated)
	}
	if !strings.HasPrefix(first.SnapshotToken, "ephemeral:") || first.Durable || first.QueryDigestSHA256 == "" || first.ObservationWatermark != 7 {
		t.Fatalf("incomplete provenance: %+v", first)
	}
	if newer.SnapshotToken == first.SnapshotToken {
		t.Fatal("observation watermark did not affect snapshot identity")
	}
	if differentQuery.SnapshotToken == first.SnapshotToken || differentQuery.QueryDigestSHA256 == first.QueryDigestSHA256 {
		t.Fatal("query did not affect provenance identity")
	}
}
