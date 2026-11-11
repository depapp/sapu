package id.sapu.app.shell

/** Result of one shell command. */
data class ShellResult(val output: String, val exitCode: Int, val timedOut: Boolean) {
    val ok: Boolean get() = exitCode == 0 && !timedOut
}

/** Something that can run commands as the shell user. */
interface Shell {
    suspend fun run(command: String, timeoutSeconds: Int = 60): ShellResult
}

/** Single-quotes a string for a POSIX shell. */
fun shQuote(s: String): String = "'" + s.replace("'", "'\\''") + "'"

internal fun parseShellOutput(raw: String): ShellResult {
    val lines = raw.trimEnd('\n').lines()
    val last = lines.lastOrNull() ?: ""
    return when {
        last == "__SAPU_TIMEOUT__" -> ShellResult(lines.dropLast(1).joinToString("\n"), -1, true)
        last.startsWith("__SAPU_EXIT__=") -> ShellResult(
            lines.dropLast(1).joinToString("\n"),
            last.removePrefix("__SAPU_EXIT__=").toIntOrNull() ?: -1,
            false,
        )
        else -> ShellResult(raw, -1, false)
    }
}
