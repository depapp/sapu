package id.sapu.app.core

import id.sapu.app.shell.Shell
import id.sapu.app.shell.shQuote

data class TrashItem(val from: String, val origin: String)
data class TrashRun(val id: String, val dir: String, val items: List<TrashItem>, val bytes: Long)

/** Reads /sdcard/.sapu/trash so runs made by the CLI and the app both show up. */
class TrashStore(private val shell: Shell) {
    suspend fun runs(): List<TrashRun> {
        val ls = shell.run("ls -1 ${shQuote(Safety.TRASH_ROOT)} 2>/dev/null")
        val out = ArrayList<TrashRun>()
        for (raw in ls.output.lines()) {
            val id = raw.trim()
            if (id.isEmpty() || id.contains("/")) continue
            val dir = Safety.TRASH_ROOT + "/" + id
            val man = shell.run("cat ${shQuote("$dir/manifest.tsv")} 2>/dev/null")
            val items = man.output.lines().mapNotNull { line ->
                val f = line.trimEnd('\r').split("\t")
                if (f.size < 2 || f[0].isEmpty()) null else TrashItem(f[0], f[1])
            }
            val size = shell.run("du -sk ${shQuote(dir)} 2>/dev/null | cut -f1", 120).output.trim().lines().firstOrNull()?.trim()?.toLongOrNull() ?: 0L
            out.add(TrashRun(id, dir, items, size * 1024))
        }
        return out.sortedByDescending { it.id }
    }

    /** Permanently deletes one run's folder. The only `rm -rf` in the app. */
    suspend fun empty(run: TrashRun): Boolean {
        if (run.id.isEmpty() || run.id.contains("/") || run.id.contains("..")) return false
        return shell.run("rm -rf ${shQuote(Safety.TRASH_ROOT + "/" + run.id)}", 300).ok
    }
}
