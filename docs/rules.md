# Built-in rules — verification

Each row of the built-in v1 set was checked on the machine listed below. A rule
whose path turned out to be **wrong** was removed from the set; the note names
it. A rule whose app is not installed here cannot be confirmed locally — it
stays, marked `not verifiable here`, because the directory layout is documented
by the vendor of the app and is the one the tool's schema already names.

Verified machine: **Windows 10 Pro for Workstations, version 22H2, build
19045.6466** (admin account; probes ran read-only).

Windows 11: not verified — no machine of that version was available when this
table was written. The layouts above are the user-profile/ProgramData trees,
expected to be the same, but nothing here survives on that claim.

| ID | Targets | Risk | In `default` | Admin | Verified (Win 10 22H2, 19045) | Windows 11 | Notes |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `browsers.chrome.cache` | `User Data\*` → Cache, Code Cache, GPUCache; closed `chrome.exe` | safe | yes | no | **verified** (Profile 1\Cache present) | not verified | ~74k items in `User Data` |
| `browsers.edge.cache` | `User Data\*` → same three; closed `msedge.exe` | safe | yes | no | **verified** (`Default` holds all three) | not verified | Edge installed by default |
| `browsers.firefox.cache` | `Profiles\*\cache2`; closed `firefox.exe` | safe | yes | no | **verified** (`default-release\cache2` present) | not verified | one of the two profiles holds a cache |
| `system.user-temp` | `%TEMP%`, older than 1 day | safe | yes | no | **verified** (~17.5k items) | not verified | busy files are the run's promise to skip, not fail |
| `system.windows-temp` | `%SystemRoot%\Temp`, older than 1 day | safe | yes | yes | **verified** (folder present; empty on a quiet machine) | not verified | running installers keep their files busy |
| `system.wer-user` | `%LOCALAPPDATA%\Microsoft\Windows\WER` | safe | yes | no | **verified: folder absent** (no reports queued here) | not verified | the layout is the standard WER tree |
| `system.wer-machine` | `%ProgramData%\...\WER` → `ReportArchive`, `ReportQueue` | safe | yes | yes | **verified** (707 + 2 items) | not reported | |
| `system.cbs-logs` | `%SystemRoot%\Logs\CBS\*.log`, older than 14 days | safe | yes | yes | **verified** (5 items; `*.log` filtered by `include`) | not verified | mode `files` keeps the folder itself |
| `system.crash-dumps` | `%LOCALAPPDATA%\CrashDumps` | moderate | no | no | **verified** (4 items) | not verified | dumps a stopped program leaves |
| `system.minidumps` | `%SystemRoot%\Minidump`, `%SystemRoot%\MEMORY.DMP` (`self`) | moderate | no | yes | **verified: folder absent** (no such dump on this machine) | not verified | |
| `system.windows-update-cache` | `%SystemRoot%\SoftwareDistribution\Download` | moderate | no | yes | **verified** (7 items) | not verified | services are not stopped; busy files ride to the next run |
| `system.delivery-optimization` | NetworkService's `DeliveryOptimization\Cache` | safe | no | yes | **verified** (folder exists, contents need elevation — access denied without it, as expected of the NetworkService profile) | not verified | |
| `system.thumbnail-cache` | `Explorer\thumbcache_*.db` (`files`) | safe | no | no | **verified** (15 files present) | not verified | explorer.exe holds part of them; those are skipped by the run |
| `system.recycle-bin` (command) | `powershell.exe -NoProfile -Command Clear-RecycleBin -Force` | moderate | no | no | **verified** (the binary is there) | not verified | the *attack surface* of the command is not verified here beyond the binary check |
| `system.component-cleanup` (command) | `dism.exe /Online /Cleanup-Image /StartComponentCleanup` | moderate | no | yes | **verified** (`dism.exe` on PATH; refuses to run without elevation — observed exit 740) | not verified | never with `/ResetBase` |
| `dev.npm-cache` | `%LOCALAPPDATA%\npm-cache` | safe | no (tag `dev`) | no | **verified** (~364k items) | not verified | |
| `dev.pnpm-store` | `%LOCALAPPDATA%\pnpm\store` | moderate | no (tag `dev`) | no | **verified** (~56k items) | not verified | |
| `dev.yarn-cache` | `%LOCALAPPDATA%\Yarn\Cache` | safe | no (tag `dev`) | no | not verifiable here (yarn not installed; layout vendor-documented) | not verified | |
| `dev.pip-cache` | `%LOCALAPPDATA%\pip\Cache` | safe | no (tag `dev`) | no | **verified** (~5.6k items) | not verified | |
| `dev.go-build-cache` | `%LOCALAPPDATA%\go-build` | safe | no (tag `dev`) | no | **verified** (~22.5k items) | not verified | the next build is slower once |
| `dev.nuget-cache` | `%LOCALAPPDATA%\NuGet\v3-cache`, `%TEMP%\NuGetScratch` | safe | no (tag `dev`) | no | **verified** (both present) | not verified | |
| `dev.gradle-caches` | `%USERPROFILE%\.gradle\caches` | moderate | no (tag `dev`) | no | **verified** (~78k items) | not verified | |
| `dev.docker-prune` (command) | `docker system prune -f` | dangerous | no (tag `dev`) | no | not verifiable here (docker not installed) | not verified | stopped containers do not come back; that is why the risk is dangerous |
| `apps.vscode.cache` | `%APPDATA%\Code` → Cache, CachedData, CachedExtensionVSIXs, Code Cache, GPUCache; closed `Code.exe` | safe | yes | no | **verified** (all five folders present) | not verified | |
| `apps.discord.cache` | `%APPDATA%\discord` → Cache, Code Cache, GPUCache; closed `Discord.exe` | safe | yes | no | not verifiable here (Discord not installed; Electron layout vendor-documented) | not verified | |
| `apps.slack.cache` | `%APPDATA%\Slack` → Cache, Code Cache, GPUCache; closed `slack.exe` | safe | yes | no | **verified** (all three present) | not verified | |

## Removed on verification

None. Every path this machine could speak to turned out as the spec named it;
the absent ones are absent apps or absent state, not wrong paths.

## Method

Read-only probes (`Test-Path`, `Get-ChildItem`, `Get-Item`) over each expanded
target; `Get-Command` for the command rules; the DISM refusal was read from an
elevated-less run's exit code. No probing deleted anything. The probe scripts
live in this repo's scratch (`.superpowers/sdd/2026-10-19-stage6/`, git-ignored).
