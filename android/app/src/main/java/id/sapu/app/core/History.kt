package id.sapu.app.core

import android.content.Context
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.io.File
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

data class HistoryEntry(val time: String, val runId: String, val command: String, val action: String, val path: String, val bytes: Long, val reason: String)

/** Append-only log of everything the funnel did, kept in app-private storage. */
class History(context: Context) {
    private val file = File(context.filesDir, "history.log")

    suspend fun append(runId: String, command: String, r: ActionResult) = withContext(Dispatchers.IO) {
        val ts = SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ss", Locale.US).format(Date())
        file.appendText(listOf(ts, runId, command, r.action, r.path, r.bytes.toString(), r.reason.replace('\t', ' ')).joinToString("\t") + "\n")
    }

    suspend fun read(limit: Int = 200): List<HistoryEntry> = withContext(Dispatchers.IO) {
        if (!file.exists()) return@withContext emptyList()
        file.readLines().asReversed().take(limit).mapNotNull { line ->
            val f = line.split("\t")
            if (f.size < 6) null else HistoryEntry(f[0], f[1], f[2], f[3], f[4], f[5].toLongOrNull() ?: 0, f.getOrElse(6) { "" })
        }
    }
}
