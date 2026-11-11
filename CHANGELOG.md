# Changelog

All notable changes to Sapu. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow [SemVer](https://semver.org). The CLI and the Android app share one version.

## [0.2.0]

### Android app
- Per-app APK, data and cache sizes plus last-opened date on the Apps tab (`dumpsys diskstats` and `dumpsys usagestats` through Shizuku), sorted by size; Home shows total app cache and data.
- Optimize tab: speed-profile compile of the 5 most launched apps and Android's background dexopt, with preview, confirmation and history; lists battery-optimization exemptions and apps unopened for 90 days. Removes nothing.
- Indonesian UI (`values-in`), chosen through the system or per-app language setting.
- Signed release APK (`sapu-android.apk`) published by the Release workflow next to the CLI binaries, with SHA256SUMS and build attestation. The workflow refuses to publish an unsigned APK.
- F-Droid metadata (English and Indonesian) under `fastlane/metadata/android`; `fdroid/id.sapu.app.yml` is the build recipe to submit to fdroiddata.
- Catalog notes, Clean section names, risk labels and skip reasons are shown in Indonesian too (shared rules carry `note_id`/`name_id`).

### CLI
- Scoop bucket for Windows: `scoop bucket add sapu https://github.com/depapp/sapu && scoop install sapu`; the manifest is bumped by the Release workflow.
- `sapu update` and `install.sh` resolve the latest release through the `releases/latest` redirect instead of the rate-limited GitHub API.
- 47 more preinstalled-app rules for Samsung, Xiaomi, OPPO, vivo and Infinix/Tecno (157 total).
- Homebrew: `brew install depapp/tap/sapu` (formula in `depapp/homebrew-tap`, bumped by the Release workflow).
- `sapu clean` shows the full plan and asks for confirmation before moving anything (skip with `--yes`); the preview applies the same protection checks as the real run.
- `sapu <command> --help` prints the usage instead of running the command.
- Indonesian text for the shared rules (`note_id`, `name_id` in the YAML), used by the Android app.

## [0.1.0]

### CLI (macOS, Linux, Windows; talks to the phone over adb, no root)
- `sapu clean`: leftovers of uninstalled apps, old installer APKs, incomplete downloads, thumbnail caches and lost clusters go to `/sdcard/.sapu/trash` with a manifest; Android's own `pm trim-caches` as the last, labelled permanent step.
- `sapu uninstall` and `--bloat`: remove apps with their `Android/data`, `obb` and `media` folders; per-vendor bloatware catalog; preinstalled apps removed for user 0 and restorable with `sapu restore`.
- `sapu analyze`: storage overview and an explorer with trash-on-select.
- `sapu status`: battery, thermal, memory, CPU, network and storage dashboard with a health score.
- `sapu optimize`: speed-profile compile of the most used apps, background dexopt, battery audit.
- `sapu probe`: anonymized device report to paste into a rules issue.
- `sapu trash`, `sapu history`, `sapu rules`, `sapu devices`, `sapu update`; `--dry-run` and `--json` on every command.
- Safety: one deletion funnel with path validation, protected packages, personal-data protection for messengers, banks, e-wallets, authenticators and password managers, whitelist, operations log, no telemetry.
- `install.sh`, README in English and Indonesian, `llms.txt` and an agent skill.

### Android app (`android/`, Kotlin + Jetpack Compose, Android 8+)
- Home, Clean (scan, preview, sweep), Apps (remove, restore) and Trash (restore, delete) on the same rules YAML and trash layout as the CLI, through Shizuku; read-only without Shizuku.
