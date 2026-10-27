# Contributing

## One rule, one file, one app

A new cleanup to `sweep` is a new YAML file, not a change in Go code. Pick the
category directory (`browsers/`, `system/`, `dev/`, `apps/`), name the file
after the app or the system area (`chrome.yaml`, `wer-user.yaml`), and fill
the template:

```yaml
schema: 1
rules:
  - id: <category>.<app>.<area>     # unique, dot-separated, lower case
    name: <App> <area>
    description: >                  # English; what is taken and what rebuilds it
      <one or two lines>
    category: <category>
    risk: safe                      # be honest: safe | moderate | dangerous
    default_enabled: true           # false for moderate and dangerous
    requires_closed: [<app>.exe]    # if the app keeps its cache busy
    targets:
      - path: '%VAR%\...'
        mode: contents
```

Read [`docs/format.md`](docs/format.md) for every field before your first rule.

## The checklist a pull request with a rule answers

1. **You verified the path on a real machine** and said which: the OS version
   and build go in `docs/rules.md`, one line per rule, next to what the probe
   saw (`verified` / `verified: folder absent` / `not verifiable here`).
   A path that turned out wrong is removed, not annotated.
2. **The risk is honest.** A cache the app rebuilds on its own is `safe`;
   something a user may still want is `dangerous`, and the default profile
   does not take it.
3. **One app per file.** Two related areas of one app (the store and the
   scratch) go as two targets of one rule; two apps are two files.
4. **The closed process is named** whenever the app keeps its cache busy,
   including enough of the executable name to be unambiguous.
5. **The guard test stays green** — nothing in the target widens past the
   narrow promise (`go test ./internal/guard/`), and the whole set still
   passes `go test ./...`.
6. **`sweep list --json` output pasted** in the pull request, so a reviewer
   sees the rule as a machine reads it, with three sample lines of the
   `description`.
7. **No rule around a tool the run must start the services of** (Windows
   Update, prefetch and the like): sweep does not stop services — the spec
   settles that, and a rule's target must live on the same page.

## Sending

Small pull requests only: one rule (or a small family) at a time, tests
green, no code changes needed. The Go code changes behind a separate pull
request, not inside the rules one.

## Trying a rule in the interface

You do not have to write YAML to test an idea. Run `sweep`, press `g`, open
`My rules` and press `n`. The form saves the rule to
`%APPDATA%\sweep\rules`, and sweep reloads it at once: scan and see what the
rule finds. `Preview YAML` shows the file the form wrote, which you can paste
into your pull request.
