---
name: sapu
description: "Drive the Sapu CLI to clean, debloat, uninstall, analyze storage and check health on an Android phone connected over adb. Use before running any `sapu` command."
when_to_use: "hp penuh, memori penuh, bersihkan hp, hapus bloatware, uninstall aplikasi android, hp lemot, apa yang memenuhi memori, phone storage full, clean my Android, debloat Samsung, debloat Xiaomi, remove preinstalled apps, what is using my phone storage, sapu"
license: GPL-3.0
---

# Sapu

Sapu runs on the user's computer and changes a phone over adb. Your job: read the
phone, show exactly what would change, hand the decision to the user.

1. **Confirm the device.** `sapu devices`. If nothing is listed, the user must enable USB
   debugging and accept the prompt on the phone; you cannot do that for them.
2. **Read before you change.** `sapu status --json`, `sapu analyze --json [path]`,
   `sapu uninstall --list --json`, `sapu uninstall --bloat --list --json`,
   `sapu history --json`, and every `--dry-run` are read-only.
3. **Preview, then consent.** Run `sapu clean --dry-run`, `sapu optimize --dry-run` or `sapu uninstall --dry-run <pkg>`,
   summarize the result (total, biggest groups, what is a leftover vs a cache), and only run
   the real command after the user says yes to that preview. `sapu clean` without a terminal
   starts immediately, so consent comes first.
4. **Interactive lists belong to the user.** `sapu uninstall` and `sapu analyze` without
   flags open a selector; give the user the command instead of driving it. Non-interactive
   removal is `sapu uninstall --yes <package...>`.
5. **Never widen.** No `adb shell rm`, `pm uninstall`, or `pm clear` for what Sapu skipped or
   protected. Preinstalled apps removed by Sapu come back with `sapu restore <package>`;
   trashed files come back with `sapu trash restore <run>`.
6. **Grow the rules.** A preinstalled app Sapu does not list is a rules gap, not something to
   remove by hand. `sapu probe` prints an anonymized report (no serial, account, user app or
   file names) the user can paste into a GitHub issue.
7. **Explain what a thing is.** Sapu prints package names, not app names. Name the app when
   you know it, mark it as your reading, and say "unknown" otherwise.
