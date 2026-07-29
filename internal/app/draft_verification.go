package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/morluto/gitcontribute/internal/contracts"
)

// VerifyPublishedDraft compares one immutable local revision with one
// explicitly synchronized GitHub thread and never performs a network read.
func (s *Service) VerifyPublishedDraft(ctx context.Context, in contracts.VerifyPublishedDraftInput) (*contracts.PublishedDraftVerification, error) {
	c, err := s.openReadOnlyCorpus(ctx)
	if err != nil {
		return nil, err
	}
	draft, err := c.GetContributionDraftRevision(ctx, in.DraftID, in.Revision)
	if err != nil {
		return nil, err
	}
	out := &contracts.PublishedDraftVerification{
		Status: "unknown", DraftID: draft.ID, Revision: draft.Revision,
		PublishedRef:     fmt.Sprintf("%s/%s#%d", in.Owner, in.Repo, in.Number),
		DraftTitleSHA256: draft.TitleSHA256, DraftBodySHA256: draft.BodySHA256, CoverageStatus: "unknown",
	}
	if in.Owner+"/"+in.Repo != draft.Repository || in.Kind != draft.Kind {
		out.Reason = "published target identity does not match the stored draft"
		return out, nil
	}
	repository, err := c.GetRepository(ctx, in.Owner, in.Repo)
	if err != nil {
		return nil, err
	}
	if repository == nil {
		out.Reason = "repository is not stored; explicitly sync the target thread"
		return out, nil
	}
	thread, err := c.GetThread(ctx, repository.ID, in.Kind, in.Number)
	if err != nil {
		return nil, err
	}
	if thread == nil || thread.UpdatedAt.IsZero() {
		out.Reason = "published thread has no source observation; explicitly sync the target thread"
		return out, nil
	}
	out.ObservedAt = formatTime(thread.UpdatedAt)
	out.SourceUpdatedAt = formatTime(thread.SourceUpdatedAt)
	out.CoverageStatus = "observed"
	if thread.UpdatedAt.Before(draft.RenderedAt) {
		out.Reason = "published observation predates this stored draft revision"
		return out, nil
	}
	out.PublishedTitleSHA256 = textSHA256(thread.Title)
	out.PublishedBodySHA256 = textSHA256(thread.Body)
	out.TitleComparison = comparePublishedText(draft.Title, thread.Title)
	out.BodyComparison = comparePublishedText(draft.Body, thread.Body)
	if out.TitleComparison == "exact_match" && out.BodyComparison == "exact_match" {
		out.Status = "exact_match"
		return out, nil
	}
	if out.TitleComparison != "mismatch" && out.BodyComparison != "mismatch" {
		out.Status = "normalized_only_match"
		return out, nil
	}
	out.Status = "mismatch"
	out.Difference = &contracts.PublishedDraftDifference{
		FirstDifferingLine: firstDifferingLine(draft.Body, thread.Body),
		DraftBytes:         len([]byte(draft.Body)), PublishedBytes: len([]byte(thread.Body)),
	}
	return out, nil
}

func comparePublishedText(left, right string) string {
	if left == right {
		return "exact_match"
	}
	normalize := func(value string) string {
		return strings.ReplaceAll(strings.ReplaceAll(value, "\r\n", "\n"), "\r", "\n")
	}
	if normalize(left) == normalize(right) {
		return "normalized_only_match"
	}
	return "mismatch"
}

func firstDifferingLine(left, right string) int {
	a, b := strings.Split(left, "\n"), strings.Split(right, "\n")
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return i + 1
		}
	}
	if len(a) != len(b) {
		if len(a) < len(b) {
			return len(a) + 1
		}
		return len(b) + 1
	}
	return 0
}

func textSHA256(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
