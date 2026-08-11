package app

import (
	"errors"
	"strings"
)

type responseFormat uint8

const (
	conciseResponse responseFormat = iota + 1
	detailedResponse
)

func parseResponseFormat(value string) (responseFormat, error) {
	switch strings.TrimSpace(value) {
	case "", "concise":
		return conciseResponse, nil
	case "detailed":
		return detailedResponse, nil
	default:
		return 0, errors.New("response_format must be concise or detailed")
	}
}

func (f responseFormat) String() string {
	if f == detailedResponse {
		return "detailed"
	}
	return "concise"
}

func (f responseFormat) includesDetails() bool {
	return f == detailedResponse
}
