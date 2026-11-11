package id.sapu.app.core

import id.sapu.app.rules.Rules
import id.sapu.app.shell.Shell
import id.sapu.app.shell.shQuote
import java.security.SecureRandom
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

data class ActionResult(
    val path: String,
    val action: String,
    val bytes: Long = 0,
    val reason: String = "",
    val trashTo: String = "",
) {
    val skipped get() = action == "SKIPPED"
}

/**
 * The single funnel every phone-changing call passes through, like
 * internal/safety.Runner in the CLI. Files go to /sdcard/.sapu/trash/<run>/
 * with a manifest the CLI can read; packages are removed for user 0 only.
 */
class Funnel(
    private val shell: Shell,
    private val rules: Rules,
    private val history: History,
    val command: String,
    val dryRun: Boolean,
    private val installed: Set<String> = emptySet(),
) {
    val runId: String = newRunId()
    private val q = ::shQuote

    suspend fun trash(p: String, sizeBytes: Long): ActionResult {
        val n = Safety.validate(p).getOrElse { return skip(p, sizeBytes, it.message ?: "invalid") }
        val owner = Safety.leftoverOwner(n)
        if (owner.isNotEmpty()) {
            val inner = n.removePrefix("/sdcard/Android/")
            val cacheLike = inner.contains("/cache") || n.contains("/files/log") || n.contains("/files/crash") ||
                n.contains("/files/tombstones") || n.endsWith(".log")
            if (owner in installed && !cacheLike) return skip(n, sizeBytes, "app is installed, not a leftover")
            if (rules.isDataProtected(owner) && n.startsWith("/sdcard/Android/media/")) {
                return skip(n, sizeBytes, "personal media of $owner")
            }
        }
        var dest = Safety.TRASH_ROOT + "/" + runId + "/" + Safety.encodeOrigin(n)
        if (dryRun) return record(ActionResult(n, "WOULD-TRASH", sizeBytes, trashTo = dest))
        val cmd = "mkdir -p ${q(Safety.TRASH_ROOT + "/" + runId)} && [ -e ${q(n)} ] && d=${q(dest)} && i=1 && " +
            "while [ -e \"\$d\" ]; do d=${q(dest)}.\$i; i=\$((i+1)); done && mv ${q(n)} \"\$d\" && printf 'moved\\t%s\\n' \"\$d\""
        val out = shell.run(cmd, 300)
        val moved = out.output.lines().map { it.trimEnd('\r') }
            .firstOrNull { it.startsWith("moved\t") && it.removePrefix("moved\t").startsWith(Safety.TRASH_ROOT + "/") }
            ?.removePrefix("moved\t")
        if (!out.ok || moved == null) return skip(n, sizeBytes, if (out.timedOut) "timed out" else "move failed")
        dest = moved
        shell.run("printf '%s\\t%s\\n' ${q(dest)} ${q(n)} >> ${q(Safety.TRASH_ROOT + "/" + runId + "/manifest.tsv")}")
        return record(ActionResult(n, "TRASHED", sizeBytes, trashTo = dest))
    }

    suspend fun uninstall(pkg: String, sizeBytes: Long = 0): ActionResult {
        if (!Safety.validPackage(pkg)) return skip(pkg, sizeBytes, "invalid package name")
        if (rules.isProtected(pkg)) return skip(pkg, sizeBytes, "protected package")
        if (dryRun) return record(ActionResult(pkg, "WOULD-UNINSTALL", sizeBytes))
        val out = shell.run("pm uninstall -k --user 0 ${q(pkg)}", 120)
        if (!out.output.contains("Success")) return skip(pkg, sizeBytes, out.output.trim().ifEmpty { "pm failed" })
        return record(ActionResult(pkg, "UNINSTALLED", sizeBytes))
    }

    suspend fun restore(pkg: String): ActionResult {
        if (!Safety.validPackage(pkg)) return skip(pkg, 0, "invalid package name")
        if (dryRun) return record(ActionResult(pkg, "WOULD-RESTORE"))
        val out = shell.run("cmd package install-existing --user 0 ${q(pkg)}", 120)
        if (!out.output.contains("installed for user")) return skip(pkg, 0, out.output.trim().ifEmpty { "restore failed" })
        return record(ActionResult(pkg, "RESTORED"))
    }

    /** Android's own permanent cache eviction. Not routed through trash. */
    suspend fun trimCaches(): ActionResult {
        if (dryRun) return record(ActionResult("pm trim-caches", "WOULD-TRIM"))
        val out = shell.run("pm trim-caches 999G", 300)
        if (!out.ok) return skip("pm trim-caches", 0, out.output.trim().ifEmpty { "failed" })
        return record(ActionResult("pm trim-caches", "TRIMMED"))
    }

    /** Ahead-of-time compiles one package with its usage profile. Nothing is removed. */
    suspend fun compile(pkg: String): ActionResult {
        if (!Safety.validPackage(pkg)) return skip(pkg, 0, "invalid package name")
        if (dryRun) return record(ActionResult(pkg, "WOULD-COMPILE"))
        val out = shell.run("cmd package compile -m speed-profile ${q(pkg)}", 300)
        if (!out.output.contains("Success")) return skip(pkg, 0, out.output.trim().ifEmpty { if (out.timedOut) "timed out" else "compile failed" })
        return record(ActionResult(pkg, "COMPILED"))
    }

    /** Runs Android's background dexopt job now instead of waiting for an idle, charging night. */
    suspend fun dexopt(): ActionResult {
        if (dryRun) return record(ActionResult("bg-dexopt-job", "WOULD-DEXOPT"))
        val out = shell.run("cmd package bg-dexopt-job", 900)
        if (!out.ok) return skip("bg-dexopt-job", 0, out.output.trim().ifEmpty { if (out.timedOut) "timed out" else "failed" })
        return record(ActionResult("bg-dexopt-job", "DEXOPTED"))
    }

    /** Moves one trashed item back to its origin; only from this run's own folder. */
    suspend fun restoreFile(runDir: String, from: String, to: String): ActionResult {
        val rest = from.removePrefix("$runDir/")
        if (!from.startsWith("$runDir/") || rest.isEmpty() || rest == "manifest.tsv" || rest.contains("/") || rest == "." || rest == "..") {
            return skip(from, 0, "not inside the run's trash")
        }
        val n = Safety.validate(to).getOrElse { return skip(to, 0, it.message ?: "invalid") }
        if (dryRun) return record(ActionResult(n, "WOULD-RESTORE", trashTo = from))
        val out = shell.run("[ -e ${q(from)} ] && [ ! -e ${q(n)} ] && mkdir -p ${q(parent(n))} && mv ${q(from)} ${q(n)} && echo ok", 300)
        if (!out.output.contains("ok")) return skip(n, 0, if (out.output.contains("No such")) "missing in trash" else "destination exists or move failed")
        return record(ActionResult(n, "RESTORED", trashTo = from))
    }

    private fun parent(p: String) = p.substringBeforeLast("/").ifEmpty { "/" }

    private suspend fun skip(p: String, size: Long, reason: String) = record(ActionResult(p, "SKIPPED", size, reason))

    private suspend fun record(r: ActionResult): ActionResult {
        history.append(runId, command, r)
        return r
    }

    companion object {
        fun newRunId(): String {
            val b = ByteArray(2).also { SecureRandom().nextBytes(it) }
            return SimpleDateFormat("yyyyMMdd-HHmmss", Locale.US).format(Date()) + "-" + b.joinToString("") { "%02x".format(it) }
        }
    }
}
