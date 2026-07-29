package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

func (r *MCPReader) AttachValidationReceipt(ctx context.Context, in mcpcontract.AttachValidationReceiptInput) (mcpcontract.ExternalValidationReceiptOutput, error) {
	decoder := json.NewDecoder(bytes.NewBufferString(in.ReceiptJSON))
	decoder.DisallowUnknownFields()
	var receipt contracts.ExternalValidationReceipt
	if err := decoder.Decode(&receipt); err != nil {
		return mcpcontract.ExternalValidationReceiptOutput{}, fmt.Errorf("decode external validation receipt: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return mcpcontract.ExternalValidationReceiptOutput{}, errors.New("external validation receipt must contain one JSON value")
	}
	result, err := r.application().AttachValidationReceipt(ctx, receipt)
	if err != nil {
		return mcpcontract.ExternalValidationReceiptOutput{}, err
	}
	out := mcpcontract.ExternalValidationReceiptOutput{
		RunID: result.ID, DefinitionID: result.DefinitionID, InvestigationID: result.InvestigationID,
		Kind: result.Kind, Classification: result.Classification,
	}
	if result.External != nil {
		out.ReceiptSHA256 = result.External.ReceiptSHA256
		out.Producer = result.External.Producer
		out.Incomplete = result.External.Incomplete
	}
	return out, nil
}

func (r *MCPReader) VerifyPublishedDraft(ctx context.Context, in mcpcontract.VerifyPublishedDraftInput) (mcpcontract.PublishedDraftVerificationOutput, error) {
	result, err := r.application().VerifyPublishedDraft(ctx, contracts.VerifyPublishedDraftInput{
		DraftID: in.DraftID, Revision: in.Revision, Owner: in.Owner, Repo: in.Repo, Kind: in.Kind, Number: in.Number,
	})
	if err != nil {
		return mcpcontract.PublishedDraftVerificationOutput{}, err
	}
	out := mcpcontract.PublishedDraftVerificationOutput{
		Status: result.Status, DraftID: result.DraftID, Revision: result.Revision, PublishedRef: result.PublishedRef,
		TitleComparison: result.TitleComparison, BodyComparison: result.BodyComparison,
		DraftTitleSHA256: result.DraftTitleSHA256, DraftBodySHA256: result.DraftBodySHA256,
		PublishedTitleSHA256: result.PublishedTitleSHA256, PublishedBodySHA256: result.PublishedBodySHA256,
		ObservedAt: result.ObservedAt, SourceUpdatedAt: result.SourceUpdatedAt, CoverageStatus: result.CoverageStatus, Reason: result.Reason,
	}
	if result.Difference != nil {
		out.Difference = &mcpcontract.PublishedDraftDifferenceOutput{
			FirstDifferingLine: result.Difference.FirstDifferingLine,
			DraftBytes:         result.Difference.DraftBytes, PublishedBytes: result.Difference.PublishedBytes,
		}
	}
	return out, nil
}
