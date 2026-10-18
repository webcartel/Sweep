# sweep

`sweep` cleans a Windows 10/11 machine according to a set of YAML rules: the
rules are data, and the Go core only scans and executes them. One binary, two
frontends — running `sweep` with no arguments opens a TUI, while `sweep scan`
and `sweep clean` work in scripts and in Task Scheduler.

## Status

**Stage 4 of 8 — the executor and `sweep clean`.** On top of the scanner,
`sweep clean` runs the plan it is agreed to: it scans, prints the plan, and
after `--yes` (or an answer at the prompt of a real terminal) takes every
selected path away — a file alone, the contents of a directory or its files,
one last guard check before every deletion, and dangerous rules excluded
unless `--include-dangerous` names them, in which case their data goes to the
recycle bin. `--dry-run` tallies what would have gone without touching the
disk; `--no-trash` prefers real deletion to the bin; `--on-reboot` postpones
the paths of busy files to the next start of Windows; a run whose log cannot
be opened does not start at all.

```sh
.\sweep.exe clean --dry-run
.\sweep.exe clean --yes --json
.\sweep.exe clean --yes --include-dangerous --no-trash=false
```

`clean` answers with the same versioned JSON as `scan` (or writes it to
`--report <file>`): what went, what stayed, why, and what waits for the next
start of Windows. The exit codes: 0 a full run, 1 an internal error or an
unopenable log, 2 a misuse of flags, 3 a confirmation that no terminal can
give, 4 a run that finished with paths left behind. Every operation is
appended to one JSONL per day under `%LOCALAPPDATA%\sweep\logs\`.

**Stages 1–3** built the grounding: `sweep scan` reads the disk and prints
what would be cleaned, without changing anything. The scanner walks each
target of a rule —
the directory itself, its contents or its files — respects the includes, the
age filters and the `min_age_days` of the rules, aggregates a directory into
one entry with the counts of what is below and never enters a reparse point.
The rules run side by side in a worker pool, the progress is throttled, and
Ctrl+C closes the stream within the second and without a plan.

```sh
.\sweep.exe scan
.\sweep.exe scan --json
.\sweep.exe scan --rule dev.*
```

`scan` prints a table (ID, items, files, size, skipped, with a TOTAL row);
`--json` prints one versioned document instead, with the whole plan of every
rule, and `--rule` takes an id or a mask of rules wherever they came from,
repeated as often as needed, taking the place of the profile.

Everything that touches WinAPI sits behind `internal/platform`: the file
system (reparse points, 8.3 names, drive kinds, the recycle bin), the process
list and the environment. The Windows side is a pair of files under a build tag,
and the tests run against an in-memory one, so the same cases run on either
machine.
`internal/guard` decides what may be touched: a path is normalized — variables,
`..`, either separator, `\\?\`, trailing dots and spaces, 8.3 names, links — and
compared against a protected list that no rule can widen. The executor holds
the same promise: it asks the guard again right before it deletes, because the
machine had time to change between the scan and the run. There is no TUI yet.

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

The rule and config parsers and the guard carry fuzz targets; a short local run
is:

```sh
go test ./internal/rules/ -run '^$' -fuzz FuzzLoadFile -fuzztime 30s
go test ./internal/guard/ -run '^$' -fuzz FuzzNormalize -fuzztime 60s
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
