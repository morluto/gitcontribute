package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

const transactionBoundReadLimitation = "transaction-bound identity; request a durable snapshot before reusing this result across calls"

func offlineReadProvenance(kind string, revision int64, input any, complete, truncated, unknownCoverage bool) (mcpcontract.CorpusReadProvenance, error) {
	query, err := json.Marshal(input)
	if err != nil {
		return mcpcontract.CorpusReadProvenance{}, fmt.Errorf("encode %s provenance input: %w", kind, err)
	}
	queryDigest := sha256.Sum256(query)
	identity, err := json.Marshal(struct {
		Contract string          `json:"contract"`
		Kind     string          `json:"kind"`
		Revision int64           `json:"observation_watermark"`
		Query    json.RawMessage `json:"query"`
	}{Contract: "corpus-read.v1", Kind: kind, Revision: revision, Query: query})
	if err != nil {
		return mcpcontract.CorpusReadProvenance{}, fmt.Errorf("encode %s provenance identity: %w", kind, err)
	}
	identityDigest := sha256.Sum256(identity)
	return mcpcontract.CorpusReadProvenance{
		SnapshotToken:        "ephemeral:" + hex.EncodeToString(identityDigest[:]),
		ObservationWatermark: revision,
		QueryDigestSHA256:    hex.EncodeToString(queryDigest[:]),
		Complete:             complete,
		Truncated:            truncated,
		UnknownCoverage:      unknownCoverage,
		Limitations:          []string{transactionBoundReadLimitation},
	}, nil
}
