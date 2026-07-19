package clustering

import (
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/similarity"
)

func duplicateThread(candidate Candidate) similarity.ThreadText {
	return similarity.ThreadText{
		Ref:   similarity.ThreadRef{Repo: candidate.Repo, Kind: domain.ThreadKind(candidate.Kind), Number: candidate.Number},
		Title: candidate.Title, Body: candidate.Body, Labels: candidate.Labels, Author: candidate.Author,
	}
}
