package discovery

import (
	"net/http"
	"time"
)

func NewArchiveClientWithOptions(baseURL string, client *http.Client, timeout time.Duration, maxBytes int64) *archiveClient {
	return newArchiveClient(baseURL, client, timeout, maxBytes)
}
