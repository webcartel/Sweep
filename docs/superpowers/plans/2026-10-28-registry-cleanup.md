# Registry Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a separate, narrow registry cleanup (`sweep registry scan|clean` and a TUI flow) for orphaned uninstall entries, dead `Run` values and dead `App Paths` keys.

**Architecture:** A read-only scanner over an abstract registry produces findings; a guard admits only three parent branches; a cleaner exports a `.reg` backup before each deletion; CLI and TUI share the same plan and result.

**Tech Stack:** Go 1.27, `golang.org/x/sys/windows/registry`, `reg.exe export`, existing Bubble Tea screens.

**Spec:** `docs/superpowers/specs/2026-10-28-registry-cleanup-design.md`

## Global Constraints

- Windows 10/11 only; the portable core must still build and test on Linux.
- No new third-party dependency: `golang.org/x/sys` is already direct.
- Never delete outside the three parent branches; never delete a key with subkeys.
- Nothing is selected by default; `clean` without `--select` refuses.
- Export failure cancels that deletion.
- All user-facing text in the tool is English; `docs/decisions.md` is Russian.

## Review Focus

- A command whose path contains spaces must not be reported dead (quote and prefix parsing).
- An unresolved `%VAR%` must be skipped as `unknown_variable`, never deleted.
- A path on a removable or network drive must be skipped as `not_local`.
- MSI entries, updates and `SystemComponent=1` must never be findings.
- A failed `.reg` export must leave the registry untouched for that finding.

---

### Task 1: Platform registry interface and fakes

**Files:**
- Create: `internal/platform/registry.go`
- Create: `internal/platform/registry_windows.go`
- Create: `internal/platform/fake/registry.go`
- Test: `internal/platform/fake/registry_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
```go
type ValueKind int
const (
    ValueString ValueKind = iota
    ValueExpandString
    ValueDWord
    ValueOther
)
type Value struct { Name, Data string; Kind ValueKind }
type Registry interface {
    SubKeys(key string) ([]string, error)
    Values(key string) ([]Value, error)
    DeleteValue(key, name string) error
    DeleteKey(key string) error
    KeyExists(key string) (bool, error)
    Export(key, path string) error
}
func NewRegistry() (Registry, error)
```
`fake.Registry` also carries `FailExport map[string]error` and `ExportCalls []string`.

- [ ] **Step 1: Write the failing fake test** — build a tree with `AddKey`/`AddValue`, assert `SubKeys` sorted, `Values` returns the default value as `""`, `DeleteKey` refuses a key with subkeys, `Export` records the call and writes the file.
- [ ] **Step 2: Run it; expect undefined symbols.**
- [ ] **Step 3: Implement the interface, the windows type (64-bit view, `WOW64_64KEY`; `Export` runs `reg.exe export <key> <path> /y`), and the fake.**
- [ ] **Step 4: Run `go test ./internal/platform/...`; green. Commit `platform: a registry behind an interface`.**

### Task 2: Findings, detection and the registry guard

**Files:**
- Create: `internal/registry/findings.go`
- Create: `internal/registry/scan.go`
- Create: `internal/registry/guard.go`
- Test: `internal/registry/scan_test.go`
- Test: `internal/registry/guard_test.go`

**Interfaces:**
- Consumes: `platform.Registry`, `platform.FS`, `platform.Env` from Task 1.
- Produces:
```go
type Kind string
const (
    KindUninstall Kind = "uninstall"
    KindRun       Kind = "run"
    KindAppPath   Kind = "apppath"
)
type Finding struct {
    Kind Kind
    ID, Name, Key, Value, Path, Reason string
    Admin, HasSubkeys, DeleteValue bool
}
func Scan(reg platform.Registry, fs platform.FS, env platform.Env) ([]Finding, []string, error)
func Guarded(key string) bool
```
`ID` is `kind + "|" + lower(key) + "|" + lower(value)`.

- [ ] **Step 1: Write failing detection tests** on `fake.Registry` plus `fake.FS`:
  - dead `UninstallString` under HKCU → one `uninstall` finding with reason `uninstall command not found: …`;
  - live path → no finding;
  - `WindowsInstaller=1`, `ParentKeyName`, `ReleaseType`, `SystemComponent=1` → no finding;
  - dead `Run` value → finding with `DeleteValue=true`; live value → none;
  - command with spaces in an unquoted path that exists → none; missing → finding;
  - `%NOPE%\app.exe` → no finding, note `unknown_variable`;
  - dead `App Paths` default value → finding; key with subkeys → `HasSubkeys=true`.
  - path on `fake.FS` drive typed `DriveRemovable`/`DriveRemote` → no finding, note `not_local`; a `DriveType` error → note `unknown_drive`.
- [ ] **Step 2: Run; red.**
- [ ] **Step 3: Implement `Scan`** over the three branches, 64-bit paths listed verbatim, skips above, env expansion via `platform.Expand`, drive kind via `fs.DriveType`, existence via `fs.Lstat`. `Guarded` admits only keys at or below the three parents, case-insensitive, accepting `HKLM`/`HKEY_LOCAL_MACHINE`.
- [ ] **Step 4: Guard test**: `HKCU\...\Run` and `HKLM\...\Uninstall\X` pass; `HKLM\SYSTEM\...`, `HKLM\...\Classes\...`, a key *above* `Run` fail.
- [ ] **Step 5: Green; commit `registry: find dead entries behind a guard`.**

### Task 3: `sweep registry scan`

**Files:**
- Create: `internal/cli/registry.go`
- Modify: `internal/cli/cli.go` (deps gains `Registry platform.Registry`; `realDeps` builds it; root adds the `registry` command)
- Test: `internal/cli/registry_test.go`

**Interfaces:**
- Consumes: `registry.Scan`, `registry.Finding`.
- Produces: `newRegistryCmd(d deps) *cobra.Command` with `scan` and `clean` (clean is Task 4).
- JSON: `{ "schema": 1, "findings": [ {"kind","id","name","key","value","path","reason","admin","has_subkeys"} ] }`.

- [ ] **Step 1: Failing CLI test** with `fake.Registry` seeded with one dead `Run` value: `registry scan` prints a table row with `run`, the value name and the reason; `--json` parses into the document above.
- [ ] **Step 2: Red; implement `scan`** (table columns KIND, NAME, KEY, REASON, ADMIN; notes to stderr).
- [ ] **Step 3: Green; commit `cli: registry scan`.**

### Task 4: Backup, clean and `sweep registry clean`

**Files:**
- Create: `internal/registry/clean.go`
- Create: `internal/registry/clean_test.go`
- Modify: `internal/cli/registry.go`
- Test: `internal/cli/registry_clean_test.go`

**Interfaces:**
- Consumes: `Finding`, `platform.Registry`.
- Produces:
```go
type Options struct {
    DryRun, Elevated bool
    BackupRoot string
    Now func() time.Time
    Log func(kind, path, note string)
}
type Entry struct { Finding Finding; Action, Note string } // action: deleted, skipped, failed, planned
type Result struct { BackupDir string; Removed, Skipped, Failed int; Entries []Entry }
func Clean(reg platform.Registry, findings []Finding, opts Options) (Result, error)
```
Backup root default `%LOCALAPPDATA%\sweep\registry-backups`, directory `YYYYMMDD-HHMMSS` (suffix `-2`, `-3` on collision), one `.reg` per key (Run value → its parent key, deduped), `manifest.json`.

- [ ] **Step 1: Failing clean tests**:
  - dry run: no `ExportCalls`, actions `planned`, no directory;
  - export error: registry untouched, action `failed`, `Failed=1`;
  - HKLM finding without elevation: `skipped`, note `needs_admin`;
  - `HasSubkeys`: `skipped`;
  - successful deletion exports first (`ExportCalls` before `DeleteValue`), report counts, manifest exists.
- [ ] **Step 2: Red; implement `Clean`** with order: guard → admin → subkeys → export all → manifest → delete.
- [ ] **Step 3: CLI test**: `registry clean` without `--select` exits 2; with `--select run:<key>:<value>` and `--yes` on a fake deletes one value, prints the backup dir; non-terminal without `--yes` exits 3; `--json` prints exactly one document with `backup_dir` and no table.
- [ ] **Step 4: Log test**: each action reaches `Options.Log`, and the CLI writes `registry_delete`, `registry_skip`, `registry_failed` lines to the JSONL log next to the file-clean log.
- [ ] **Step 5: Integration test** behind `SWEEP_REGISTRY_INTEGRATION=1`: create `HKCU\Software\sweep-test` with one dead value, scan it, export, delete, restore from the `.reg`, and remove the test key.
- [ ] **Step 6: Green; commit `registry: clean with a .reg backup before every delete`.**

### Task 5: The TUI flow

**Files:**
- Create: `internal/tui/regclean.go`
- Modify: `internal/tui/tui.go` (Deps gains `Registry platform.Registry`, `RegistryPreselect []string`, `RestartRegistry func(ids []string) error`; screens and switch)
- Modify: `internal/tui/profile.go` (button `Registry cleanup`)
- Modify: `internal/cli/cli.go` (wire the new deps and a hidden `--preselect-registry`)
- Test: `internal/tui/regclean_test.go`

**Interfaces:**
- Consumes: `registry.Scan`, `registry.Clean`, existing frame helpers.
- Produces: screens `regScan`, `regReview`, `regConfirm`, `regRunning`, `regDone`; `regState` with `findings []registry.Finding`, `selected map[string]bool`, `result registry.Result`.

- [ ] **Step 1: Failing tests**: start button opens the flow; review starts with nothing selected; Space toggles a finding; confirm runs a fake clean; done shows the backup dir; `[A]` findings survive `--preselect-registry`.
- [ ] **Step 2: Red; implement screens**: scan progress, review table (KIND, NAME, REASON, `[A]`), confirm restating counts, running, done. No "Select safe only".
- [ ] **Step 3: Green; commit `tui: registry cleanup`.**

### Task 6: Documentation and the read-only check

**Files:**
- Create: `docs/registry.md`
- Modify: `README.md` (a Registry cleanup section and command list)
- Modify: `docs/decisions.md` (a Russian section with the calls of the design)

- [ ] **Step 1: `docs/registry.md`** — what is cleaned, what is not, the backup location and restore steps, the `--select` format.
- [ ] **Step 2: README** — commands and one screenshot of the review screen.
- [ ] **Step 3: decisions** — narrow set, `not_local`, `--preselect-registry`, spec remains v1.
- [ ] **Step 4: Read-only check** — `sweep registry scan` and `sweep registry scan --json` on this machine; record the counts in the ledger. No `clean` on the real machine.
- [ ] **Step 5: Full suite, vet, lint; commit `docs: registry cleanup`.**
