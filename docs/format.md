# The rule format

Every rule is one YAML file under a category directory
(`rules/browsers/`, `rules/system/`, `rules/dev/`, `rules/apps/`, or a
directory of your own). The JSON schema the parsers validate against is
[`schema/rule.schema.json`](../schema/rule.schema.json); a file carries
`schema: 1`.

The full field set, on one made-up rule:

```yaml
schema: 1
rules:
  - id: dev.example-cache            # unique across every file sweep reads
    name: Example cache              # one screen of English
    description: What the run takes away and what the app rebuilds, on its own, once.
    category: dev                    # what `sweep list --category dev` filters by
    risk: safe                       # safe | moderate | dangerous
    default_enabled: true            # the default profile takes it when true
    requires_admin: false            # only elevated runs select it
    requires_closed: [MyApp.exe]     # the run skips the rule while this is running
    os_min_build: 17763              # machines older than this build never select it
    trash: false                     # true sends the rule's paths to the bin, always
    tags: [dev]                      # the `dev` profile selects tags like this one
    targets:
      - path: '%LOCALAPPDATA%\Example\Data\*\Cache'
        mode: contents               # contents | self | files
        include: ['*.dat']           # with mode: files: what counts as a file
        exclude: ['keep*']           # never removed, at any mode
        min_age_days: 7              # atime/mtime younger than this: skipped
        min_size: 25MB               # smaller files stay
      - kind: command                # the other kind of target
        cmd: powershell.exe          # one binary, no shell; argv follows
        args: ['-NoProfile', '-Command', 'Clear-RecycleBin -Force']
```

## The pieces the examples key off

### Path patterns

A `path` target starts with an environment variable or an absolute path and
may carry `*` masks. `%WINDIR%`-style variables are expanded with the machine's
environment before the guard runs, so a rule never names a machine's own
layout. One `*` matches one directory level; the pattern that ends mid-name
(`thumbcache_*.db`) matches files.

### Modes

- `contents` — what is inside the matched directories; the directory itself
  stays. The default.
- `self` — the matched object itself: a file or a directory.
- `files` — the files that match `include` under the matched directory.

`contents` aimed at a single file simply skips it; this is what lets a
temporary-file rule and a loose-dump rule share one shape.

### The runs' guard

A target is re-checked against the protected-path list before every strip: if
a rule names more than the narrow path it promises (say, `%SystemRoot%`
itself), the guard refuses the run rather than the deletion widening. A rule
cannot opt out of this.

### Risk

- `safe` — the app rebuilds it on its own; in the default profile by default.
- `moderate` — comes back, at a cost (a slower next build or a next download);
  out of the default profile, one toggle away.
- `dangerous` — a user may still want it; out unless named, and even when
  toggled in, a run asks for the word `yes` before it goes (CLI:
  `--include-dangerous`).

### Commands

`kind: command` names a binary and its argv, no shell in between. If the
binary is not on PATH, the run ends the rule as `failed` with the
`executable file not found` error — a rule that does not always have its tool
on the machine is better off at `moderate` risk and out of the default
profile. A `requires_admin` command rule is left for the run the user
elevates.

## Where rules live

The binary carries the same files this repository publishes
(`go:embed`), so a rule cannot drift from the binary that ships it. User rules
land in `%APPDATA%\sweep\rules\*.yaml` and **replace a built-in rule of the
same id** (a warning prints); `--rules-dir` adds extra directories that win
over both.

## Editing rules in the interface

Press `g` on the start screen, then `My rules`, then `n`. The form writes the
fields above; `Preview YAML` shows the whole file before it is saved. The save
runs `LoadFile`, `ValidateSchema` and `ValidateSemantics`, exactly the checks
the CLI runs, so a file the interface accepts is a file `sweep validate`
accepts too.

Three limits keep the form honest:

- The form holds one rule with one path target. A file with several rules or a
  `kind: command` target opens in Notepad instead.
- A built-in rule cannot be edited. Write your own rule with the same id to
  override it; the merge prints a warning.
- Every save and delete leaves a `.bak` copy next to the file.

