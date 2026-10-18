# sweep

`sweep` cleans a Windows 10/11 machine according to a set of YAML rules: the
rules are data, and the Go core only scans and executes them. One binary, two
frontends — running `sweep` with no arguments opens a TUI, while `sweep scan`
and `sweep clean` work in scripts and in Task Scheduler.

## Status

**Stage 3 of 8 — the scanner.** Everything the previous stages built now
answers a real question: `sweep scan` reads the disk and prints what would be
cleaned, without changing anything. The scanner walks each target of a rule —
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
compared against a protected list that no rule can widen. There is no cleaner
and no TUI yet.

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
