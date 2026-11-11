package id.sapu.app.core

import id.sapu.app.shell.Shell
import kotlinx.coroutines.delay
import java.text.SimpleDateFormat
import java.util.Locale

data class SizeEntry(val app: Long, val data: Long, val cache: Long) {
    val total: Long get() = app + data + cache
}

data class Usage(val lastUsedMs: Long, val launchCount: Int)

/**
 * Per-app sizes and usage, read through the shell the same way the CLI does
 * (internal/device/packages.go): dumpsys diskstats, usagestats and deviceidle.
 */
class Stats(private val shell: Shell) {
    /**
     * Sizes from `dumpsys diskstats`. When [refresh] is true (or no data exists
     * yet) the system's own DiskStatsLoggingService job is run first, which is
     * how Settings > Storage gets its numbers.
     */
    suspend fun sizes(refresh: Boolean): Map<String, SizeEntry> {
        val before = shell.run("dumpsys diskstats", 30).output
        val old = parseDiskstats(before)
        if (!refresh && old != null) return old
        val job = shell.run("dumpsys jobscheduler | grep -m1 'DiskStatsLoggingService#'", 30).output
        JOB_RE.find(job)?.let { shell.run("cmd jobscheduler run -f android ${it.groupValues[1]}", 30) }
        val deadline = System.currentTimeMillis() + 45_000
        while (System.currentTimeMillis() < deadline) {
            delay(2_000)
            val now = shell.run("dumpsys diskstats", 30).output
            if (now != before) parseDiskstats(now)?.let { return it }
        }
        // The job did not report within the deadline (or nothing changed): keep what we have.
        return old ?: throw IllegalStateException("Per-app sizes are not available yet, try again in a minute")
    }

    suspend fun usage(): Map<String, Usage> = parseUsage(shell.run(USAGE_CMD, 120).output)

    suspend fun batteryExempt(): List<String> =
        parseDeviceIdleWhitelist(shell.run("dumpsys deviceidle whitelist", 30).output)

    companion object {
        private val JOB_RE = Regex("""android/com\.android\.server\.storage\.DiskStatsLoggingService#(\d+)""")

        // dumpsys usagestats is hundreds of KB; reduce it to "pkg|lastTimeUsed|launches"
        // rows in the shell so the result fits in one binder transaction.
        internal val USAGE_CMD = """dumpsys usagestats | sed -n """ +
            """-e 's/.*package=\([^ ]*\) .*lastTimeUsed="\([^"]*\)".*appLaunchCount=\([0-9]*\).*/\1|\2|\3/p' """ +
            """-e 's/.*package=\([^ ]*\) .*lastTimeUsed=\([0-9][0-9]*\) .*/\1|\2|0/p' | sort -u | head -c 800000"""

    }
}

internal fun parseDiskstats(out: String): Map<String, SizeEntry>? {
    var names: List<String> = emptyList()
    var app: List<Long> = emptyList()
    var data: List<Long> = emptyList()
    var cache: List<Long> = emptyList()
    for (line in out.lineSequence()) {
        val i = line.indexOf(':')
        if (i < 0) continue
        val v = line.substring(i + 1).trim()
        when (line.substring(0, i).trim()) {
            "Package Names" -> names = jsonStrings(v)
            "App Sizes" -> app = jsonLongs(v)
            "App Data Sizes" -> data = jsonLongs(v)
            "Cache Sizes" -> cache = jsonLongs(v)
        }
    }
    if (names.isEmpty()) return null
    return names.mapIndexed { i, n ->
        n to SizeEntry(app.getOrElse(i) { 0 }, data.getOrElse(i) { 0 }, cache.getOrElse(i) { 0 })
    }.toMap()
}

private fun jsonStrings(v: String) =
    v.removePrefix("[").removeSuffix("]").split(',').map { it.trim().removeSurrounding("\"") }.filter { it.isNotEmpty() }

private fun jsonLongs(v: String) =
    v.removePrefix("[").removeSuffix("]").split(',').mapNotNull { it.trim().toLongOrNull() }

/** Parses the rows produced by [Stats.USAGE_CMD]; keeps the latest time and highest launch count per package. */
internal fun parseUsage(out: String): Map<String, Usage> {
    val fmt = SimpleDateFormat("yyyy-MM-dd HH:mm:ss", Locale.US)
    val res = HashMap<String, Usage>()
    for (line in out.lineSequence()) {
        val f = line.trim().split('|')
        if (f.size < 3 || f[0].isEmpty()) continue
        val raw = f[1]
        var ms = if (raw.length >= 10 && raw.all { it.isDigit() }) raw.toLongOrNull() ?: 0L
        else try { fmt.parse(raw)?.time ?: 0L } catch (e: Exception) { 0L }
        if (ms < 365L * 86_400_000) ms = 0L // 1970 means never used
        val launches = f[2].toIntOrNull() ?: 0
        val u = res[f[0]] ?: Usage(0, 0)
        res[f[0]] = Usage(maxOf(u.lastUsedMs, ms), maxOf(u.launchCount, launches))
    }
    return res
}

/** Packages the user exempted from battery optimization: rows "user,<pkg>,<uid>". */
internal fun parseDeviceIdleWhitelist(out: String): List<String> =
    out.lineSequence().map { it.trim().split(',') }
        .filter { it.size >= 2 && it[0] == "user" && it[1].isNotEmpty() }
        .map { it[1] }.toList()
