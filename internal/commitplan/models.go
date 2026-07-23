// Package commitplan builds verifiable, read-only semantic commit plans from
// Git-owned patches. It never stages files or changes repository history.
package commitplan

// Snapshot is the immutable input observed from one workspace.
type Snapshot struct {
	Patch     []byte
	Untracked []UntrackedFile
}

// UntrackedFile identifies exact untracked content without embedding it.
type UntrackedFile struct {
	Path     string
	ObjectID string
}

// Unit is the smallest assignable file or hunk change.
type Unit struct {
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	Path           string `json:"path"`
	OldPath        string `json:"old_path,omitempty"`
	Operation      string `json:"operation"`
	OldStart       int32  `json:"old_start,omitempty"`
	OldLines       int32  `json:"old_lines,omitempty"`
	NewStart       int32  `json:"new_start,omitempty"`
	NewLines       int32  `json:"new_lines,omitempty"`
	Patch          string `json:"patch,omitempty"`
	ContentHash    string `json:"content_sha256"`
	Generated      bool   `json:"generated"`
	WhitespaceOnly bool   `json:"whitespace_only"`
}

// Warning describes a condition requiring human or agent judgment.
type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Path    string `json:"path,omitempty"`
	UnitID  string `json:"unit_id,omitempty"`
}

// Inventory is a bounded, deterministic view of one source patch.
type Inventory struct {
	Units             []Unit    `json:"units"`
	Warnings          []Warning `json:"warnings,omitempty"`
	SourcePatchSHA256 string    `json:"source_patch_sha256"`
	InventorySHA256   string    `json:"inventory_sha256"`
}

// GroupInput supplies semantic judgment while deterministic code verifies
// coverage and dependencies.
type GroupInput struct {
	Name               string
	Intent             string
	Type               string
	Scope              string
	UnitIDs            []string
	DependsOn          []string
	ValidationCommands []string
	TestOwners         []string
}

// UnresolvedInput explains why a unit cannot yet be assigned safely.
type UnresolvedInput struct {
	UnitID string
	Reason string
}

// PlanInput is the agent-authored semantic layer over a frozen inventory.
type PlanInput struct {
	Groups     []GroupInput
	Unresolved []UnresolvedInput
}

// Group is one proposed semantic commit.
type Group struct {
	Name               string   `json:"name"`
	Intent             string   `json:"intent"`
	SuggestedSubject   string   `json:"suggested_subject"`
	UnitIDs            []string `json:"unit_ids"`
	Files              []string `json:"files"`
	DependsOn          []string `json:"depends_on,omitempty"`
	ValidationCommands []string `json:"validation_commands,omitempty"`
	TestOwners         []string `json:"test_owners,omitempty"`
}

// Unresolved records an unassigned or ambiguous unit.
type Unresolved struct {
	UnitID string `json:"unit_id"`
	Reason string `json:"reason"`
}

// Reconstruction binds exact source bytes to one-to-one unit coverage.
type Reconstruction struct {
	SourcePatchSHA256 string `json:"source_patch_sha256"`
	InventorySHA256   string `json:"inventory_sha256"`
	AssignedSHA256    string `json:"assigned_sha256"`
	UnitCount         int    `json:"unit_count"`
	AssignedCount     int    `json:"assigned_count"`
	Verified          bool   `json:"verified"`
}

// Plan is a read-only semantic proposal plus deterministic coverage proof.
type Plan struct {
	Groups         []Group        `json:"groups"`
	Unresolved     []Unresolved   `json:"unresolved,omitempty"`
	Warnings       []Warning      `json:"warnings,omitempty"`
	Reconstruction Reconstruction `json:"reconstruction"`
}
