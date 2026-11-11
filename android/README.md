# Sapu for Android

Native companion to the `sapu` CLI. Same rules database (`../internal/rules/data`, added as Gradle assets), same safety funnel (`core/Funnel.kt`), same trash layout under `/sdcard/.sapu/trash`.

```bash
./gradlew assembleDebug        # app/build/outputs/apk/debug/app-debug.apk
./gradlew testDebugUnitTest    # path rules and rules-database tests
```

Needs JDK 17 and an Android SDK with platform 34 (`local.properties` or `ANDROID_HOME`).

## How it gets shell access

Privileged work runs in `shell/ShellService.kt`, a Shizuku UserService started by Shizuku as uid 2000. The app side (`shell/ShizukuBridge.kt`) tracks four states: Shizuku not installed, not running, permission needed, ready. Every destructive action goes through `core/Funnel.kt`: path validation (`core/Safety.kt`), protected and data-protected rules, trash with manifest, `pm uninstall -k --user 0` and `cmd package install-existing`.

Testing on an emulator without the Shizuku UI: install the Shizuku APK, then start the server from adb with the starter that ships inside it:

```bash
APK=$(adb shell pm path moe.shizuku.privileged.api | sed 's/package://')
adb shell "$(dirname $APK)/lib/x86_64/libshizuku.so"
```

## Release signing

`assembleRelease` reads the signing key from the environment, so the repository never carries a keystore:

| Variable | Meaning |
| --- | --- |
| `SAPU_KEYSTORE_FILE` | path to a `.jks` keystore |
| `SAPU_KEYSTORE_PASSWORD` | keystore password |
| `SAPU_KEY_ALIAS` | key alias inside the keystore |
| `SAPU_KEY_PASSWORD` | password of that key |

Without `SAPU_KEYSTORE_FILE` the release build is unsigned, which is fine for local checks. The Release workflow refuses to publish an unsigned APK: it needs the four values as GitHub Actions secrets, with the keystore stored base64-encoded as `SAPU_KEYSTORE_BASE64`.

Create a key once and keep the keystore somewhere safe (losing it means users cannot update in place):

```bash
keytool -genkeypair -v -keystore sapu-release.jks -alias sapu -keyalg RSA -keysize 4096 -validity 10000
gh secret set SAPU_KEYSTORE_BASE64 < <(base64 -w0 sapu-release.jks)
gh secret set SAPU_KEYSTORE_PASSWORD
gh secret set SAPU_KEY_ALIAS --body sapu
gh secret set SAPU_KEY_PASSWORD
```

## Languages

UI strings live in `app/src/main/res/values/strings.xml` (English) and `values-in/strings.xml` (Indonesian). Android picks the one matching the system or per-app language.
