# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

All commands run from `src/`:

```bash
# Run during development
go run main.go

# macOS: suppress -lobjc duplicate linker warning
CGO_LDFLAGS="-Wl,-w" go run main.go

# Build check (no output binary)
go build ./...

# Package into distributable app with icon
go run fyne.io/tools/cmd/fyne@latest package -name FrameFit -icon icon.png

# Tidy dependencies
go mod tidy
```

There are no tests at the moment.

## Architecture

Single-file Go app (`src/main.go`) with a [Fyne](https://fyne.io/) GUI. The cascade classifier binary (`src/facefinder`) is embedded at compile time via `//go:embed`.

### Processing pipeline

`onStartPressed` → `runProcessing` (goroutine) → per-image goroutines (semaphore-limited to `maxConcurrentImages = 5`) → `processImage`

`processImage` branches on aspect ratio:
- **Portrait** (`imgH >= imgW`) → `processPortrait`: blurred+darkened fill as background, fitted+feathered foreground overlaid center
- **Landscape** → `processLandscape`: tries `detectFaces` first; if faces found uses `faceAwareCrop` (centers on face bounding box); otherwise falls back to `smartcrop`

### Cancellation

`runProcessing` accepts a `context.Context`. The Stop button calls `cancelFn()`. Each goroutine checks `ctx.Err()` before processing. The UI goroutine checks `ctx.Err()` after `wg.Wait()` to distinguish stopped vs completed.

### UI updates from goroutines

Progress and status are updated via `widget.SetText` / `widget.SetValue` called from the background goroutine. Fyne v2 widget setters are safe to call from any goroutine.

### Key constraints
- Source and destination must differ (enforced in `onStartPressed`)
- Dimensions capped at `maxDimension = 10000` px
- Output directory structure mirrors source (relative paths preserved)
- `src/framefit` is the compiled dev binary — do not commit it

## Release

Push a `vX.Y.Z` tag. GitHub Actions (`.github/workflows/build.yml`) builds for all three platforms and publishes a release with zipped artifacts automatically.
