# Registry cleanup

`sweep registry` removes three kinds of dead entries that Windows leaves
behind. It is a separate, narrow set on purpose: most "registry cleaner"
findings are harmless, and some are dangerous to touch.

## What is cleaned

| Kind | Where | Finding |
| --- | --- | --- |
| `uninstall` | `HKCU`/`HKLM` `...\CurrentVersion\Uninstall\*`, plus `WOW6432Node` | the uninstall command points at a file that does not exist |
| `run` | `HKCU`/`HKLM` `...\CurrentVersion\Run`, plus `WOW6432Node` | the autostart program does not exist. Only the value is removed |
| `apppath` | `HKCU`/`HKLM` `...\CurrentVersion\App Paths\*`, plus `WOW6432Node` | the default program path does not exist |

## What is never touched

- COM/CLSID, shared DLLs, file extensions and associations.
- Services, policies, `HKLM\SYSTEM`, `SAM`, `SECURITY`, `Classes`.
- MUI cache, icons, fonts and help files.
- MSI entries (`WindowsInstaller=1`) and updates (`ParentKeyName`,
  `ReleaseType`).
- Keys marked `SystemComponent=1`.
- Keys with subkeys.
- Paths on removable or network drives: the drive may come back.

## Commands

```sh
sweep registry scan            # read-only table
sweep registry scan --json     # one versioned document
sweep registry clean --select "run:HKEY_CURRENT_USER\...\Run:MyApp" --yes
sweep registry clean --dry-run # the plan, change nothing
```

`--select` naming:

```
kind:key[:value]
```

- `kind` is `uninstall`, `run` or `apppath`.
- `key` is the full key path, backslashes as they are.
- `value` is required for `run` and names the value; the rest of the text
  after the second colon belongs to the value.
- Comparison ignores case.

Without `--select`, `clean` refuses. Without a terminal and `--yes`, it
returns exit code 3. `--dry-run` needs no choice and ticks nothing.

## Backups and restore

Before the first deletion, every touched key is exported:

```
%LOCALAPPDATA%\sweep\registry-backups\YYYYMMDD-HHMMSS\
  backup-01.reg
  manifest.json
```

`manifest.json` maps a finding to its `.reg` file. To restore an entry,
double-click the file and agree to the import. A failed export cancels the
deletion of that key: there is no path where an unbacked-up key goes.

## In the interface

The start screen has `Registry cleanup`. The flow is Read → Review → Confirm
→ Remove → Done. Nothing is ticked at the start. Findings that need
administrator rights are marked `[A]`; the confirmation offers
`Restart as administrator`, and the choice comes back after the restart.

![The review screen of the registry cleanup](.github/registry.png)

## Verification

The scanner is read-only and was run on the development machine; it found four
candidates. No deletion was run on the real machine. The cleaner's export,
delete and restore path is covered by an opt-in test:

```sh
SWEEP_REGISTRY_INTEGRATION=1 go test ./internal/registry/ -run TestCleanRealHive
```

The test creates `HKCU\...\App Paths\sweep-test.exe`, exports it, deletes it,
restores it from the `.reg` file, and removes the key.
