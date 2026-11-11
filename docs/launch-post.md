# Sapu: sweep your Android clean from the terminal or a native app, free and open source

*Launch post draft (English). Short version for X/Threads/Reddit titles at the bottom.*

If you liked [Mole](https://github.com/tw93/mole), the "free CleanMyMac" that went viral on GitHub, this is the Android version: **Sapu**.

Sapu is Indonesian for broom. It sweeps an Android phone clean: leftover folders of apps you uninstalled long ago, old installer APKs, half-finished downloads, thumbnail caches, every app's cache, and the preinstalled bloatware from Samsung, Xiaomi, OPPO, vivo, Infinix and Tecno that Settings will not let you remove. No root, no ads, no "upgrade to premium".

## Why this exists

Since Android 11 the cleaner apps on the Play Store cannot do much. They may not read other apps' `Android/data` folders, may not clear other apps' caches and may not remove preinstalled apps. So most of them are a rocket animation plus ads.

The thing that can do all of that is **adb**, the debugging bridge Android developers use every day. Sapu is built on it: from your computer over USB, or from the phone itself through [Shizuku](https://shizuku.rikka.app) (one-time setup, no root).

## Two ways to use it

**CLI** (macOS, Linux, Windows), one binary, a terminal UI that is pleasant to look at:

```bash
brew install depapp/tap/sapu   # or: curl -fsSL https://raw.githubusercontent.com/depapp/sapu/main/install.sh | sh
sapu                     # interactive menu
sapu clean               # sweep
sapu uninstall --bloat   # pick the bloatware to remove
sapu analyze             # what is eating your storage
sapu status              # battery, temperature, RAM, CPU, network
sapu optimize            # compile your most used apps, dexopt, battery audit
```

**Android app** (APK, Kotlin + Jetpack Compose, English and Indonesian): Home, Clean, Apps, Optimize and Trash tabs. Needs Shizuku to sweep and remove; without it you still get storage and the app list.

Both read the same rules database and write to the same trash folder, so what you swept from the app can be restored from the terminal, and the other way round.

## Safe first, clean second

This is the part I spent the most time on, because deleting other people's files is serious:

- **Preview first.** Every command has `--dry-run` (in the app: a Preview button). You see exactly what will happen before anything changes.
- **Everything goes to trash.** Files are moved to `/sdcard/.sapu/trash` with a manifest, not deleted. `sapu trash restore` brings them back. The only permanent step is Android's own cache eviction (the same thing as Settings > Storage > Clear cache), and it is labelled as such.
- **Preinstalled apps are not really gone.** They are removed for the current user only; `sapu restore` brings them back any time.
- **Your data is protected.** WhatsApp and Telegram media, banking apps, e-wallets, authenticators and password managers are never touched, even when you remove the app.
- **Protected list.** Dialer, SMS, Play Services, keyboard, launcher and the other system components are refused no matter what you type.
- **One deletion funnel.** Every destructive operation goes through one function with path validation, and CI rejects code that calls `rm` or `pm uninstall` anywhere else.
- **History, no telemetry.** `sapu history` records everything that happened. Nothing is sent anywhere.

## Help the rules grow

The bloatware database is young: tested on the emulator plus public lists. If you own a Samsung, Xiaomi, OPPO, vivo, Infinix or Tecno phone, run `sapu probe` and paste the output into an issue. It contains only the phone model, Android version and the preinstalled packages Sapu does not know yet: no serial number, accounts, personal app names or file names.

Repo: https://github.com/depapp/sapu (GPL-3.0). A star helps other people find it.

---

## Short version

Built a "Mole" for Android: **Sapu**, an open source cleaner and debloater, no root.

From the terminal (`sapu clean`, `sapu uninstall --bloat`, `sapu analyze`, `sapu status`, `sapu optimize`) or from a native Android app through Shizuku.

Everything previews first, everything goes to a restorable trash, WhatsApp and banking data are never touched, no ads, no telemetry.

https://github.com/depapp/sapu

## Title ideas

- Show HN: Sapu, an open-source Mole-style cleaner and debloater for Android (adb/Shizuku, no root)
- r/androiddev: I built an open-source Android cleaner that works without root: one Go CLI over adb plus a Compose app over Shizuku, same rules database
- r/Android: Sapu: sweep leftovers, caches and OEM bloatware off your phone, with preview and a restorable trash
