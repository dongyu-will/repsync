# Contributing to repsync

Thanks for helping improve repsync. It is an early-stage local desktop tool; focused, reviewable changes are the easiest to maintain.

## Getting started

Follow the [README](README.md#quick-start) to install dependencies and launch the desktop app. Browser preview is useful for interface work but cannot validate native file writes, processes, dialogs, or CLI control. The Go and frontend MyGo versions must stay aligned.

Read [AGENTS.md](AGENTS.md) before working in this repository. [PRODUCT.md](PRODUCT.md) owns product decisions and [DESIGN.md](DESIGN.md) owns interface conventions. Both currently contain Chinese development notes; ask for clarification if a requirement is unclear.

## Issues and pull requests

For a bug, include the operating system and architecture, commit or release version, reproduction steps, expected and actual behavior, and relevant redacted logs. For interface issues, include the window size and a screenshot if useful. Remove paths, credentials, and session content you do not intend to share publicly.

Discuss larger features or new dependencies in an issue first. Reuse the existing modules and UI components; keep changes specific to the problem. Avoid incidental formatting, new compatibility layers without consumers, or additional test frameworks. The project uses MyGo, not Wails, and Tailwind CSS rather than a second styling system.

Use English for new public documentation and interface copy. Do not translate user-owned documents, command output, or session quotes. Keep the Chinese reference guide aligned when changing commands or configuration. Never hand-edit `frontend/src/mygo.ts`; regenerate bindings with `go tool mygo generate`.

## Validation

Use the [README validation commands](README.md#development-and-validation) as the maintained command reference. Run the frontend checks for interface changes, and the affected Go integration checks for backend changes. Integration tests must operate on temporary directories and repositories, never on a contributor's registered projects or personal AI sessions.

Before requesting review, run all checks required by CI. For a desktop behavior change, state what you verified in the native app. For layout changes, check the documented window sizes and keyboard behavior. Do not present browser fixture results as native integration evidence.

A pull request should explain the problem, resulting behavior, validation performed, and any remaining limitation. Separate local checks, packaging, publication, and live verification. Do not include generated builds, local history, logs, credentials, or unrelated project changes.

## License

By submitting a contribution, you agree that it may be distributed under the repository's MIT license. Preserve third-party notices when adding or modifying vendored material.
