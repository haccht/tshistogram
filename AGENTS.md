# Repository Guidelines

## Project Structure & Module Organization
This repository is a small Go CLI application. [`main.go`](/Users/thachimu/src/github.com/haccht/tshistogram/main.go) contains the command entrypoint, flag parsing, timestamp parsing, and histogram rendering logic. [`main_test.go`](/Users/thachimu/src/github.com/haccht/tshistogram/main_test.go) holds unit tests for parsing behavior. [`README.md`](/Users/thachimu/src/github.com/haccht/tshistogram/README.md) documents user-facing usage, and [`.goreleaser.yaml`](/Users/thachimu/src/github.com/haccht/tshistogram/.goreleaser.yaml) defines release packaging. Keep new code close to the CLI unless the file becomes large enough to justify extracting a focused package.

## Build, Test, and Development Commands
Use the standard Go toolchain:

- `go build -o tshistogram .` builds the local binary.
- `go run . -h` prints the CLI help and validates flag wiring.
- `go test ./...` runs the full test suite.
- `gofmt -w main.go main_test.go` formats the current codebase before review.

When changing output examples or flags, update the README in the same change.

## Coding Style & Naming Conventions
Follow idiomatic Go. Let `gofmt` control formatting; do not hand-align code. Use tabs as emitted by `gofmt`, camelCase for internal identifiers, and descriptive test names such as `TestParseLeadingTimeWithSeparator`. Prefer table-driven tests for parsing and formatting branches. Keep flag names lowercase and consistent with existing short/long options such as `-f` and `--format`.

## Testing Guidelines
Tests use Go’s built-in `testing` package. Add coverage for new timestamp formats, separator handling, and edge cases in auto-detection. Keep tests deterministic by constructing explicit `time.Time` values instead of relying on local clock state. Run `go test ./...` before opening a PR; add focused `t.Run(...)` subtests when expanding parser behavior.

## Commit & Pull Request Guidelines
Recent history uses short, imperative commit subjects such as `Add tests for timestamp parsing` and `Update README application options`. Follow that style, keep commits scoped, and separate refactors from behavior changes when possible. PRs should explain the user-visible change, mention any flag or output format updates, and include example CLI input/output when terminal rendering changes. Confirm tests pass locally before requesting review.

## Release & Documentation Notes
If you add flags or alter output, update both [`README.md`](/Users/thachimu/src/github.com/haccht/tshistogram/README.md) and [`.goreleaser.yaml`](/Users/thachimu/src/github.com/haccht/tshistogram/.goreleaser.yaml) when packaging behavior is affected.
