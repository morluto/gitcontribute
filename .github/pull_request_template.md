## Description

<!-- What does this change do and why? -->

## Testing Done

<!-- How was this change tested? Include commands and scenarios. -->

- [ ] `go test ./...` passes
- [ ] `golangci-lint run ./...` passes
- [ ] `go test -race ./internal/app ./internal/corpus` passes

## Checklist

- [ ] Focused regression tests added for ordering, resumability, cancellation, and side-effect boundaries
- [ ] Storage invariants preserved (see CONTRIBUTING.md)
- [ ] `gofmt` applied to changed Go files
- [ ] No new third-party types exposed outside adapters
