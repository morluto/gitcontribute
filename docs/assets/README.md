# TUI captures

These hero images are captured from the production `gitcontribute` executable
at 118 by 36 terminal cells against a deterministic temporary corpus:

- `gitcontribute-tui-workbench.png` — main three-pane contribution workbench
- `gitcontribute-tui-research-brief.png` — full-screen candidate research brief

Both PNGs are 1311 by 833 pixels. To rebuild them locally:

```sh
UPDATE_TUI_HERO=1 go test ./internal/app -run TestCaptureTUIHeroes -count=1
```

The capture test requires `tmux`, `agg`, and ImageMagick's `magick` command. It
builds and drives the real executable in an isolated home directory; it does
not contact GitHub or mutate the user's corpus.
