package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

type mergeState uint8

const (
	mergeUnknown mergeState = iota
	mergeObservedUnmerged
	mergeObservedMerged
)

// MergeStatus is an observed pull-request merge outcome. Its zero value means
// that merge state has not been observed.
type MergeStatus struct {
	state mergeState
	at    time.Time
}

// UnknownMergeStatus returns a merge status that has not been observed.
func UnknownMergeStatus() MergeStatus { return MergeStatus{} }

// UnmergedStatus returns an observed, unmerged status.
func UnmergedStatus() MergeStatus { return MergeStatus{state: mergeObservedUnmerged} }

// MergedStatus returns an observed, merged status. Some providers establish
// the outcome without returning its timestamp, so at may be zero.
func MergedStatus(at time.Time) MergeStatus {
	return MergeStatus{state: mergeObservedMerged, at: at}
}

// ParseMergeStatus reparses persisted scalar columns into a consistent status.
func ParseMergeStatus(known, merged bool, at time.Time) (MergeStatus, error) {
	switch {
	case !known && (merged || !at.IsZero()):
		return MergeStatus{}, errors.New("unknown merge status cannot be merged or have a merge time")
	case !known:
		return UnknownMergeStatus(), nil
	case !merged && !at.IsZero():
		return MergeStatus{}, errors.New("unmerged status cannot have a merge time")
	case merged:
		return MergedStatus(at), nil
	default:
		return UnmergedStatus(), nil
	}
}

// Known reports whether the merge outcome was observed.
func (s MergeStatus) Known() bool { return s.state != mergeUnknown }

// IsMerged reports whether the observed outcome is merged.
func (s MergeStatus) IsMerged() bool { return s.state == mergeObservedMerged }

// MergedAt returns the observed merge time, if the provider supplied one.
func (s MergeStatus) MergedAt() time.Time { return s.at }

// Equal compares parsed merge observations without exposing their representation.
func (s MergeStatus) Equal(other MergeStatus) bool { return s == other }

type mergeStatusJSON struct {
	Known    bool
	Merged   bool
	MergedAt time.Time
}

func (s MergeStatus) MarshalJSON() ([]byte, error) {
	return json.Marshal(mergeStatusJSON{Known: s.Known(), Merged: s.IsMerged(), MergedAt: s.MergedAt()})
}

func (s *MergeStatus) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var stored mergeStatusJSON
	if err := decoder.Decode(&stored); err != nil {
		return fmt.Errorf("decode merge status: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("decode merge status: expected one JSON value")
	}
	parsed, err := ParseMergeStatus(stored.Known, stored.Merged, stored.MergedAt)
	if err != nil {
		return err
	}
	*s = parsed
	return nil
}
