package id.sapu.app.core

import id.sapu.app.rules.Rules
import id.sapu.app.shell.Shell
import id.sapu.app.shell.shQuote

data class Candidate(
    val path: String,
    val bytes: Long,
    val section: String,
    val owner: String = "",
    val note: String = "",
    val skip: String = "",
)

/** Finds what `clean` may trash. Same rules and shell probes as the CLI. */
class Scanner(private val shell: Shell, private val rules: Rules) {
    private val packageRe = Regex("^[A-Za-z][A-Za-z0-9_]*(\\.[A-Za-z0-9_]+)+$")

    suspend fun scan(installed: Set<String>): List<Candidate> {
        val left = leftovers(installed)
        val clean = cleanable(installed)
        val parents = left.filter { it.skip.isEmpty() }.map { it.path }
        val nested = clean.filter { c -> parents.none { c.path.startsWith("$it/") } }
        return left + nested
    }

    private suspend fun leftovers(installed: Set<String>): List<Candidate> {
        val out = ArrayList<Candidate>()
        for (dir in listOf("/sdcard/Android/data", "/sdcard/Android/obb", "/sdcard/Android/media")) {
            val list = shell.run("ls -1 ${shQuote(dir)} 2>/dev/null")
            for (raw in list.output.lines()) {
                val name = raw.trim()
                if (name.isEmpty() || name in installed || !packageRe.matches(name)) continue
                val p = "$dir/$name"
                if (rules.isDataProtected(name) && dir.endsWith("/media")) {
                    out.add(Candidate(p, du(p), "Leftovers", note = "personal media", skip = "personal media of $name"))
                    continue
                }
                val bytes = du(p)
                val skip = if (bytes == 0L && !readable(p)) "not readable by shell" else ""
                out.add(Candidate(p, bytes, "Leftovers", owner = name, skip = skip))
            }
        }
        for (l in rules.leftovers) {
            if (l.pkg == "__never__" || l.pkg in installed || rules.isDataProtected(l.pkg)) continue
            val p = "/sdcard/" + l.path
            val exists = shell.run("[ -e ${shQuote(p)} ] && echo yes")
            if (!exists.output.contains("yes")) continue
            out.add(Candidate(p, du(p), "Leftovers", owner = l.pkg))
        }
        return out.sortedByDescending { it.bytes }
    }

    private suspend fun cleanable(installed: Set<String>): List<Candidate> {
        val out = ArrayList<Candidate>()
        val seen = HashSet<String>()
        val running = HashMap<String, Boolean>()
        for (rule in rules.cleanable) {
            val glob = "/sdcard/" + rule.glob
            val age = if (rule.minAgeDays > 0) " -mtime +${rule.minAgeDays - 1}" else ""
            val test = if (rule.kind == "dir") "-d" else "-f"
            val type = if (rule.kind == "dir") "d" else "f"
            val cmd = "for p in $glob; do [ $test \"\$p\" ] && find \"\$p\" -maxdepth 0 -type $type$age 2>/dev/null; done"
            val list = shell.run(cmd, 30)
            for (raw in list.output.lines()) {
                val p = raw.trim()
                if (p.isEmpty() || p in seen || p.contains("*")) continue
                seen.add(p)
                val owner = Safety.leftoverOwner(p)
                if (owner.isNotEmpty() && rule.runningGuard && owner in installed) {
                    val isRunning = running.getOrPut(owner) { shell.run("pidof ${shQuote(owner)} >/dev/null && echo yes").output.contains("yes") }
                    if (isRunning) {
                        out.add(Candidate(p, du(p), rule.displayName, owner, rule.displayNote, skip = "$owner is running"))
                        continue
                    }
                }
                val bytes = du(p)
                if (bytes == 0L && rule.kind == "dir") continue
                out.add(Candidate(p, bytes, rule.displayName, owner, rule.displayNote))
            }
        }
        return out
    }

    suspend fun du(p: String): Long {
        val out = shell.run("du -sk ${shQuote(p)} 2>/dev/null | cut -f1", 120)
        return (out.output.trim().lines().firstOrNull()?.trim()?.toLongOrNull() ?: 0L) * 1024
    }

    private suspend fun readable(p: String) = shell.run("[ -r ${shQuote(p)} ] && echo yes").output.contains("yes")
}
