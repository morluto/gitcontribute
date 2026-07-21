package cli

// ValidationExpectedObservation is one assertion over captured output or a declared artifact.
type ValidationExpectedObservation struct {
	Name       string `json:"name"`
	Source     string `json:"source"`
	Matcher    string `json:"matcher"`
	Pattern    string `json:"pattern"`
	Occurrence string `json:"occurrence"`
	Path       string `json:"path,omitempty"`
}

// ValidationObservationContract ties output assertions to a proof intent.
type ValidationObservationContract struct {
	Intent    string                          `json:"intent"`
	Base      []ValidationExpectedObservation `json:"base,omitempty"`
	Candidate []ValidationExpectedObservation `json:"candidate,omitempty"`
}

// ValidationObservationResult records one evaluated output assertion.
type ValidationObservationResult struct {
	ValidationExpectedObservation
	Status  string `json:"status"`
	Excerpt string `json:"excerpt,omitempty"`
	Error   string `json:"error,omitempty"`
}
