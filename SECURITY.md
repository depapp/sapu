# Security

Sapu changes files and apps on a phone, so safety comes before features. This page says what Sapu refuses to do, how deletions are routed, and how to report a problem.

## Boundaries

- Sapu runs on your computer and talks to the phone through `adb` as the `shell` user. It never asks for root and never installs anything on the phone.
- The app cache eviction in `sapu clean` is Android's own `pm trim-caches` (what Settings > Storage > Clear cache runs). It is permanent, clears running apps' caches too, and is labelled that way in normal and `--dry-run` output.
- Every file deletion goes through one funnel, `internal/safety`, which validates the path (absolute, inside shared storage, no `..`, no control bytes, never a storage root such as `/sdcard/DCIM` or `/sdcard/Android/data`), then **moves** the item into `/sdcard/.sapu/trash/<run>/` instead of deleting it. `sapu trash restore <run>` puts everything back; `sapu trash empty` is the only permanent deletion and asks first.
- Apps are removed with `pm uninstall --user 0`. Preinstalled apps keep their APK (`-k`) and can be reinstalled with `sapu restore`. Packages in `protected_packages.yaml` (framework, Play services, keyboards, dialers, launchers, OEM permission managers) are refused even when selected.
- Shared-storage folders of apps in `data_protected.yaml` (chat apps, banks, e-wallets, authenticators, password managers, photo editors) are never trashed automatically, even as leftovers of an uninstalled app.
- Leftovers are matched by exact package name (`Android/{data,obb,media}/<package>` and the `leftovers.yaml` table), never by guessing from a display name.
- `--dry-run` exists on every command that changes the phone and prints exactly what the real run would do.
- A raw `rm`, `pm uninstall`, `pm clear` or `find -delete` outside `internal/safety` fails `go test ./...` unless the line carries a `// SAFE: <reason>` annotation (`internal/safety/audit_test.go`).
- Every action is appended to `~/.config/sapu/history.log` on the computer; `sapu history` shows it. Set `SAPU_NO_HISTORY=1` to disable.
- No telemetry. Sapu makes no network requests except `sapu update` (not yet shipped).

## Not defended

- A user who edits the rules files or runs `adb shell rm` themselves.
- A phone where `adb` itself is compromised.
- Data an app stores in private storage (`/data/data`): Sapu cannot read it and only the system's own `pm trim-caches` touches caches there.

## Reporting

Open a GitHub issue for anything that is not sensitive. For a vulnerability (for example a path that passes validation and should not), email the maintainer via the address on the GitHub profile and include the `sapu --version` output and the history rows involved.
