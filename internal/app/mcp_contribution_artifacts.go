package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

func (r *MCPReader) ImportExternalEvidenceManifest(ctx context.Context, in mcpcontract.ImportExternalEvidenceManifestInput) (mcpcontract.ImportExternalEvidenceManifestOutput, error) {
	decoder := json.NewDecoder(bytes.NewBufferString(in.ManifestJSON))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var input contracts.ExternalEvidenceManifest
	if err := decoder.Decode(&input); err != nil {
		return mcpcontract.ImportExternalEvidenceManifestOutput{}, fmt.Errorf("decode external evidence manifest: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return mcpcontract.ImportExternalEvidenceManifestOutput{}, errors.New("external evidence manifest must contain one JSON value")
	}
	claims := make([]evidence.ExternalEvidenceClaim, 0, len(input.Claims))
	for _, claim := range input.Claims {
		claims = append(claims, evidence.ExternalEvidenceClaim{ID: claim.ID, Type: evidence.EvidenceType(claim.Type), Relation: evidence.Relation(claim.Relation), Description: claim.Description, SourceRefs: append([]string(nil), claim.SourceRefs...), Measurements: claim.Measurements})
	}
	result, err := r.application().importExternalEvidenceManifest(ctx, evidence.ExternalEvidenceManifest{
		SchemaVersion: input.SchemaVersion, Producer: input.Producer, InvestigationID: input.InvestigationID, HypothesisID: input.HypothesisID, OpportunityID: input.OpportunityID,
		Repository: input.Repository, Revision: input.Revision, ArtifactSHA256: input.ArtifactSHA256, ObservedAt: input.ObservedAt, Environment: input.Environment,
		Completeness: input.Completeness, Integrity: input.Integrity, Limitations: input.Limitations, Claims: claims, ManifestSHA256: input.ManifestSHA256,
	})
	if err != nil {
		return mcpcontract.ImportExternalEvidenceManifestOutput{}, err
	}
	return mcpcontract.ImportExternalEvidenceManifestOutput{ManifestSHA256: result.ManifestSHA256, Producer: result.Producer, ClaimCount: result.ClaimCount, Incomplete: result.Incomplete}, nil
}

func (r *MCPReader) AttachJUnitReport(ctx context.Context, in mcpcontract.AttachJUnitReportInput) (mcpcontract.AttachJUnitReportOutput, error) {
	c, err := r.openCorpus(ctx)
	if err != nil {
		return mcpcontract.AttachJUnitReportOutput{}, err
	}
	runID := strings.TrimSpace(in.RunID)
	run, parseErr := evidence.NewService(c, evidence.NewExecRunner()).AttachJUnitReport(ctx, runID, strings.NewReader(in.ReportXML), evidence.JUnitParseOptions{})
	if run == nil {
		return mcpcontract.AttachJUnitReportOutput{}, parseErr
	}
	out := mcpcontract.AttachJUnitReportOutput{RunID: run.ID}
	if run.JUnitReport != nil {
		report := run.JUnitReport
		out.Schema, out.RawSHA256, out.Incomplete, out.ParseError = report.SchemaVersion, report.RawSHA256, report.Incomplete, report.ParseError
		out.Total, out.Passed, out.Failed, out.Skipped, out.Errored, out.Unknown = report.Counts.Total, report.Counts.Passed, report.Counts.Failed, report.Counts.Skipped, report.Counts.Errored, report.Counts.Unknown
	}
	if parseErr != nil && !errors.Is(parseErr, evidence.ErrJUnitReportIncomplete) {
		return out, parseErr
	}
	return out, nil
}

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
		out.Provider = result.External.Provider
		out.ExternalRunID = result.External.ExternalRunID
		out.SourceRevision = result.External.Revision
		out.ArtifactSHA256 = result.External.ArtifactSHA256
		out.Limitations = append([]string(nil), result.External.Limitations...)
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
