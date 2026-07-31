package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const maxRepositoryImportBytes = 4 << 20

func readRepositoryImport(reader io.Reader) ([]string, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxRepositoryImportBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read repository import: %w", err)
	}
	if len(data) > maxRepositoryImportBytes {
		return nil, fmt.Errorf("repository import exceeds %d bytes", maxRepositoryImportBytes)
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil, errors.New("repository import is empty")
	}
	if strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "{") {
		return parseRepositoryImportJSON([]byte(trimmed))
	}
	var refs []string
	scanner := bufio.NewScanner(strings.NewReader(trimmed))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			refs = append(refs, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan repository import: %w", err)
	}
	return refs, nil
}

func parseRepositoryImportJSON(data []byte) ([]string, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, errors.New("repository import is empty")
	}
	var items []json.RawMessage
	if data[0] == '[' {
		if err := json.Unmarshal(data, &items); err != nil {
			return nil, fmt.Errorf("parse repository import: %w", err)
		}
	} else {
		var wrapper struct {
			Repositories []json.RawMessage `json:"repositories"`
		}
		if err := json.Unmarshal(data, &wrapper); err != nil {
			return nil, fmt.Errorf("parse repository import: %w", err)
		}
		items = wrapper.Repositories
	}
	refs := make([]string, 0, len(items))
	for _, item := range items {
		var text string
		if json.Unmarshal(item, &text) == nil {
			refs = append(refs, text)
			continue
		}
		var object struct {
			Owner    string `json:"owner"`
			Repo     string `json:"repo"`
			FullName string `json:"full_name"`
			URL      string `json:"url"`
		}
		if err := json.Unmarshal(item, &object); err != nil {
			return nil, fmt.Errorf("parse repository import item: %w", err)
		}
		switch {
		case object.Owner != "" && object.Repo != "":
			refs = append(refs, object.Owner+"/"+object.Repo)
		case object.FullName != "":
			refs = append(refs, object.FullName)
		case object.URL != "":
			refs = append(refs, object.URL)
		default:
			return nil, errors.New("repository import item requires owner/repo, full_name, or url")
		}
	}
	if len(refs) == 0 {
		return nil, errors.New("repository import contains no repositories")
	}
	return refs, nil
}
