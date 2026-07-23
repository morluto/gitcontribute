package commitplan

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sourcegraph/go-diff/diff"
)

const (
	maxPatchBytes    = 4 << 20
	maxUnits         = 2000
	maxGroups        = 100
	maxGroupMetadata = 100
	maxPlanTextBytes = 256 << 10
)

var conventionalTypes = map[string]struct{}{
	"build": {}, "chore": {}, "ci": {}, "docs": {}, "feat": {}, "fix": {},
	"perf": {}, "refactor": {}, "revert": {}, "style": {}, "test": {},
}

// Inspect parses one bounded Git patch with a maintained unified-diff parser.
func Inspect(ctx context.Context, snapshot Snapshot) (Inventory, error) {
	if err := ctx.Err(); err != nil {
		return Inventory{}, err
	}
	if len(snapshot.Patch) > maxPatchBytes {
		return Inventory{}, fmt.Errorf("workspace patch exceeds %d bytes", maxPatchBytes)
	}
	if len(snapshot.Untracked) > maxUnits {
		return Inventory{}, fmt.Errorf("workspace diff exceeds %d untracked files", maxUnits)
	}
	files, err := parsePatch(snapshot.Patch)
	if err != nil {
		return Inventory{}, fmt.Errorf("parse workspace patch: %w", err)
	}
	result := Inventory{SourcePatchSHA256: digest(snapshot.Patch)}
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return Inventory{}, err
		}
		units, warnings, err := fileUnits(file)
		if err != nil {
			return Inventory{}, err
		}
		result.Units = append(result.Units, units...)
		result.Warnings = append(result.Warnings, warnings...)
	}
	for _, file := range snapshot.Untracked {
		if err := ctx.Err(); err != nil {
			return Inventory{}, err
		}
		path, objectID := strings.TrimSpace(file.Path), strings.TrimSpace(file.ObjectID)
		if path == "" || objectID == "" {
			return Inventory{}, errors.New("untracked path and object ID are required")
		}
		unit := newUnit("untracked", path, "", "add", []byte(path+"\x00"+objectID))
		unit.ContentHash = objectID
		unit.Generated = generatedPath(path)
		result.Units = append(result.Units, unit)
		result.Warnings = append(result.Warnings, Warning{Code: "untracked_file", Message: "untracked content is one indivisible unit", Path: path, UnitID: unit.ID})
	}
	if len(result.Units) > maxUnits {
		return Inventory{}, fmt.Errorf("workspace diff exceeds %d assignable units", maxUnits)
	}
	result.InventorySHA256 = unitDigest(result.Units, nil)
	return result, nil
}

func parsePatch(patch []byte) ([]*diff.FileDiff, error) {
	reader := diff.NewMultiFileDiffReaderOptions(bytes.NewReader(patch), diff.ParseOptions{KeepCR: true})
	var files []*diff.FileDiff
	for {
		file, trailing, err := reader.ReadFileWithTrailingContent()
		if file != nil {
			files = append(files, file)
		}
		if strings.TrimSpace(trailing) != "" {
			return nil, errors.New("workspace patch contains unparsed trailing content")
		}
		if errors.Is(err, io.EOF) {
			return files, nil
		}
		if err != nil {
			return nil, fmt.Errorf("parse workspace patch: %w", err)
		}
	}
}

func fileUnits(file *diff.FileDiff) ([]Unit, []Warning, error) {
	path, oldPath := cleanDiffPath(file.NewName), cleanDiffPath(file.OrigName)
	if path == "" || path == "/dev/null" {
		path = oldPath
	}
	operation := fileOperation(file)
	generated := generatedPath(path)
	var units []Unit
	metadata := strings.Join(file.Extended, "\n") + "\n" + file.OrigName + "\n" + file.NewName
	if operation != "modify" || len(file.Hunks) == 0 {
		unit := newUnit("file", path, oldPath, operation, []byte(metadata))
		unit.Generated = generated
		units = append(units, unit)
	}
	for _, hunk := range file.Hunks {
		printed, err := diff.PrintHunks([]*diff.Hunk{hunk})
		if err != nil {
			return nil, nil, fmt.Errorf("render hunk for %s: %w", path, err)
		}
		identity := []byte(path + "\x00" + oldPath + "\x00" + operation + "\x00" + string(printed))
		unit := newUnit("hunk", path, oldPath, operation, identity)
		unit.OldStart, unit.OldLines = hunk.OrigStartLine, hunk.OrigLines
		unit.NewStart, unit.NewLines = hunk.NewStartLine, hunk.NewLines
		unit.Patch = string(printed)
		unit.Generated = generated
		unit.WhitespaceOnly = whitespaceOnly(hunk.Body)
		units = append(units, unit)
	}
	var warnings []Warning
	if generated {
		warnings = append(warnings, Warning{Code: "generated_file", Message: "generated or snapshot file needs regeneration ownership", Path: path})
	}
	if operation == "rename" || operation == "copy" {
		warnings = append(warnings, Warning{Code: operation, Message: operation + " metadata is an indivisible file unit", Path: path})
	}
	if binaryFile(file) {
		warnings = append(warnings, Warning{Code: "binary_file", Message: "binary change is one indivisible file unit", Path: path})
	}
	for _, unit := range units {
		if unit.WhitespaceOnly {
			warnings = append(warnings, Warning{Code: "formatting_only", Message: "hunk changes only whitespace", Path: path, UnitID: unit.ID})
		}
	}
	return units, warnings, nil
}

func newUnit(kind, path, oldPath, operation string, identity []byte) Unit {
	hash := digest(identity)
	return Unit{ID: kind + ":" + hash, Kind: kind, Path: path, OldPath: oldPath, Operation: operation, ContentHash: hash}
}

func cleanDiffPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "/dev/null" {
		return path
	}
	if strings.HasPrefix(path, "a/") || strings.HasPrefix(path, "b/") {
		return path[2:]
	}
	return path
}

func fileOperation(file *diff.FileDiff) string {
	extended := strings.Join(file.Extended, "\n")
	switch {
	case strings.Contains(extended, "rename from "):
		return "rename"
	case strings.Contains(extended, "copy from "):
		return "copy"
	case file.OrigName == "/dev/null" || strings.Contains(extended, "new file mode "):
		return "add"
	case file.NewName == "/dev/null" || strings.Contains(extended, "deleted file mode "):
		return "delete"
	case binaryFile(file):
		return "binary"
	case strings.Contains(extended, "old mode ") && strings.Contains(extended, "new mode "):
		return "mode"
	default:
		return "modify"
	}
}

func binaryFile(file *diff.FileDiff) bool {
	extended := strings.Join(file.Extended, "\n")
	return strings.Contains(extended, "GIT binary patch") || strings.Contains(extended, "Binary files ")
}

func generatedPath(path string) bool {
	lower := strings.ToLower(filepath.ToSlash(path))
	base := filepath.Base(lower)
	return strings.Contains(lower, "/generated/") || strings.Contains(lower, "/snapshots/") ||
		strings.HasSuffix(lower, ".snap") || strings.HasSuffix(lower, ".golden") ||
		strings.HasSuffix(base, ".generated.go") || strings.HasSuffix(base, "_generated.go")
}

func whitespaceOnly(body []byte) bool {
	var removed, added []string
	for _, line := range strings.Split(string(body), "\n") {
		if len(line) == 0 {
			continue
		}
		switch line[0] {
		case '-':
			removed = append(removed, strings.Join(strings.Fields(line[1:]), ""))
		case '+':
			added = append(added, strings.Join(strings.Fields(line[1:]), ""))
		}
	}
	return len(removed) > 0 && len(added) > 0 && strings.Join(removed, "\n") == strings.Join(added, "\n")
}

// Build validates semantic groups against a frozen inventory.
func Build(ctx context.Context, inventory Inventory, input PlanInput) (Plan, error) {
	if err := ctx.Err(); err != nil {
		return Plan{}, err
	}
	if len(input.Groups) > maxGroups || len(input.Unresolved) > maxUnits {
		return Plan{}, errors.New("semantic commit plan exceeds group or unresolved-unit bounds")
	}
	units := make(map[string]Unit, len(inventory.Units))
	for _, unit := range inventory.Units {
		units[unit.ID] = unit
	}
	groups, names, assigned, warnings, err := buildGroups(ctx, units, input.Groups)
	if err != nil {
		return Plan{}, err
	}
	if err := validateDependencies(groups, names); err != nil {
		return Plan{}, err
	}
	unresolved, err := resolveUnassigned(inventory.Units, units, assigned, input.Unresolved)
	if err != nil {
		return Plan{}, err
	}
	result := Plan{Groups: groups, Unresolved: unresolved, Warnings: append(append([]Warning(nil), inventory.Warnings...), warnings...)}
	result.Warnings = append(result.Warnings, mixedFileWarnings(inventory.Units, assigned)...)
	assignedDigest := unitDigest(inventory.Units, assigned)
	result.Reconstruction = Reconstruction{
		SourcePatchSHA256: inventory.SourcePatchSHA256, InventorySHA256: inventory.InventorySHA256,
		AssignedSHA256: assignedDigest, UnitCount: len(inventory.Units), AssignedCount: len(assigned),
		Verified: len(assigned) == len(inventory.Units) && assignedDigest == inventory.InventorySHA256,
	}
	return result, nil
}

func buildGroups(ctx context.Context, units map[string]Unit, candidates []GroupInput) ([]Group, map[string]struct{}, map[string]string, []Warning, error) {
	names := make(map[string]struct{}, len(candidates))
	assigned := make(map[string]string, len(units))
	var groups []Group
	var warnings []Warning
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return nil, nil, nil, nil, err
		}
		group, err := buildGroup(candidate, units, assigned)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		if _, exists := names[group.Name]; exists {
			return nil, nil, nil, nil, fmt.Errorf("duplicate commit group %q", group.Name)
		}
		names[group.Name] = struct{}{}
		if len(group.SuggestedSubject) > 72 {
			warnings = append(warnings, Warning{Code: "long_subject", Message: "suggested commit subject exceeds 72 characters"})
		}
		groups = append(groups, group)
	}
	return groups, names, assigned, warnings, nil
}

func buildGroup(candidate GroupInput, units map[string]Unit, assigned map[string]string) (Group, error) {
	name, intent, kind := strings.TrimSpace(candidate.Name), strings.TrimSpace(candidate.Intent), strings.TrimSpace(candidate.Type)
	if name == "" || intent == "" || len(candidate.UnitIDs) == 0 {
		return Group{}, errors.New("each commit group requires name, intent, and unit IDs")
	}
	if len(candidate.UnitIDs) > maxUnits || len(candidate.DependsOn) > maxGroups || len(candidate.ValidationCommands) > maxGroupMetadata || len(candidate.TestOwners) > maxGroupMetadata {
		return Group{}, fmt.Errorf("group %q exceeds assignment or metadata bounds", name)
	}
	if len(name)+len(intent)+len(candidate.Scope)+joinedBytes(candidate.UnitIDs)+joinedBytes(candidate.DependsOn)+joinedBytes(candidate.ValidationCommands)+joinedBytes(candidate.TestOwners) > maxPlanTextBytes {
		return Group{}, fmt.Errorf("group %q text exceeds 256 KiB", name)
	}
	if _, valid := conventionalTypes[kind]; !valid {
		return Group{}, fmt.Errorf("group %q has unsupported conventional commit type %q", name, kind)
	}
	group := Group{Name: name, Intent: intent, SuggestedSubject: conventionalSubject(kind, candidate.Scope, intent), DependsOn: unique(candidate.DependsOn), ValidationCommands: unique(candidate.ValidationCommands), TestOwners: unique(candidate.TestOwners)}
	fileSet := map[string]struct{}{}
	for _, id := range unique(candidate.UnitIDs) {
		unit, exists := units[id]
		if !exists {
			return Group{}, fmt.Errorf("group %q references unknown unit %q", name, id)
		}
		if previous := assigned[id]; previous != "" {
			return Group{}, fmt.Errorf("unit %q is assigned to both %q and %q", id, previous, name)
		}
		assigned[id] = name
		group.UnitIDs = append(group.UnitIDs, id)
		fileSet[unit.Path] = struct{}{}
	}
	for path := range fileSet {
		group.Files = append(group.Files, path)
	}
	sort.Strings(group.Files)
	return group, nil
}

func resolveUnassigned(inventory []Unit, units map[string]Unit, assigned map[string]string, requested []UnresolvedInput) ([]Unresolved, error) {
	reasons := make(map[string]string, len(requested))
	textBytes := 0
	for _, item := range requested {
		id, reason := strings.TrimSpace(item.UnitID), strings.TrimSpace(item.Reason)
		textBytes += len(id) + len(reason)
		if textBytes > maxPlanTextBytes {
			return nil, errors.New("unresolved-unit text exceeds 256 KiB")
		}
		if _, exists := units[id]; !exists {
			return nil, fmt.Errorf("unresolved entry references unknown unit %q", id)
		}
		if assigned[id] != "" {
			return nil, fmt.Errorf("unit %q is both assigned and unresolved", id)
		}
		if reason == "" {
			return nil, fmt.Errorf("unresolved unit %q requires a reason", id)
		}
		reasons[id] = reason
	}
	var unresolved []Unresolved
	for _, unit := range inventory {
		if assigned[unit.ID] == "" {
			reason := reasons[unit.ID]
			if reason == "" {
				reason = "not assigned"
			}
			unresolved = append(unresolved, Unresolved{UnitID: unit.ID, Reason: reason})
		}
	}
	return unresolved, nil
}

func mixedFileWarnings(units []Unit, assigned map[string]string) []Warning {
	fileGroups := map[string]map[string]struct{}{}
	for _, unit := range units {
		group := assigned[unit.ID]
		if group == "" {
			continue
		}
		if fileGroups[unit.Path] == nil {
			fileGroups[unit.Path] = map[string]struct{}{}
		}
		fileGroups[unit.Path][group] = struct{}{}
	}
	var warnings []Warning
	paths := make([]string, 0, len(fileGroups))
	for path := range fileGroups {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		owners := fileGroups[path]
		if len(owners) > 1 {
			warnings = append(warnings, Warning{Code: "mixed_file", Message: "file hunks span multiple commit groups", Path: path})
		}
	}
	return warnings
}

func validateDependencies(groups []Group, names map[string]struct{}) error {
	dependencies := make(map[string][]string, len(groups))
	for _, group := range groups {
		for _, dependency := range group.DependsOn {
			if dependency == group.Name {
				return fmt.Errorf("group %q cannot depend on itself", group.Name)
			}
			if _, exists := names[dependency]; !exists {
				return fmt.Errorf("group %q depends on unknown group %q", group.Name, dependency)
			}
		}
		dependencies[group.Name] = group.DependsOn
	}
	visiting, visited := map[string]bool{}, map[string]bool{}
	var visit func(string) error
	visit = func(name string) error {
		if visiting[name] {
			return fmt.Errorf("commit group dependency cycle includes %q", name)
		}
		if visited[name] {
			return nil
		}
		visiting[name] = true
		for _, dependency := range dependencies[name] {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		visiting[name], visited[name] = false, true
		return nil
	}
	for name := range names {
		if err := visit(name); err != nil {
			return err
		}
	}
	return nil
}

func conventionalSubject(kind, scope, intent string) string {
	prefix := kind
	if scope = strings.TrimSpace(scope); scope != "" {
		prefix += "(" + scope + ")"
	}
	return prefix + ": " + strings.TrimSpace(intent)
}

func unique(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func joinedBytes(values []string) int {
	total := 0
	for _, value := range values {
		total += len(value)
	}
	return total
}

func unitDigest(units []Unit, assigned map[string]string) string {
	hash := sha256.New()
	for _, unit := range units {
		if assigned != nil && assigned[unit.ID] == "" {
			continue
		}
		_, _ = hash.Write([]byte(unit.ID))
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func digest(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
