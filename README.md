# sweep

`sweep` cleans a Windows 10/11 machine according to a set of YAML rules: the
rules are data, and the Go core only scans and executes them. One binary, two
frontends — running `sweep` with no arguments opens a TUI, while `sweep scan`
and `sweep clean` work in scripts and in Task Scheduler.

## Status

**Stage 1 of 8 — rules.** The repository builds, `sweep version` prints the
version, and CI runs on `windows-latest` and `ubuntu-latest`. The rules are
data: `sweep list` shows what was loaded and whether the profile takes it,
`sweep rules validate` checks rule files or the directories that hold them and
reports every finding as `file:line:column` on the error stream, and `sweep
rules show <id>` prints one merged rule with its `%VARIABLES%` expanded. A rule
of the user replaces the built-in one of the same id. There is no scanner, no
cleaner and no TUI yet.

```sh
.\sweep.exe list
.\sweep.exe list --json --profile dev
.\sweep.exe rules validate
.\sweep.exe rules show browsers.chrome.cache
```

The rules and the config live in `%APPDATA%\sweep`: `rules\*.yaml` and
`config.yaml`; `--rules-dir` adds a directory of rule files and `--config`
points the config elsewhere.

The specification lives in [`docs/sweep-spec.md`](docs/sweep-spec.md) (Russian).
It is the authority on behaviour; its «Этапы реализации» section lists the
eight stages and the "done when" criterion of each.

## Build and test

Requires Go 1.27 or newer.

```sh
go build ./...
go vet ./...
go test ./...
```

The rule and config parsers carry fuzz targets; a short local run is:

```sh
go test ./internal/rules/ -run '^$' -fuzz FuzzLoadFile -fuzztime 30s
```

Build the binary and print its version:

```sh
go build -o sweep.exe ./cmd/sweep
.\sweep.exe version
```

## Design in one paragraph

Rules are data: a new cleanup is a new YAML file, not a change in Go code. A
plan always comes before deletion, and `--dry-run` changes nothing. Safety
lives in the core — the protected-path list is checked before every deletion
and a rule cannot bypass it. The CLI and the TUI share one set of types and
one event stream, and everything that touches WinAPI sits behind
`internal/platform`, so the scanner and the executor are tested on fakes.

## License

MIT — see [LICENSE](LICENSE).
