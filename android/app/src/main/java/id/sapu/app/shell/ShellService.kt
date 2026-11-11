package id.sapu.app.shell

import java.io.File
import java.util.concurrent.TimeUnit
import kotlin.system.exitProcess

/**
 * Runs inside the process Shizuku starts for us (uid 2000, the same user the
 * adb shell has). It is the only place in the app that executes shell text.
 */
class ShellService : IShellService.Stub() {
    override fun run(command: String, timeoutSeconds: Int): String {
        val proc = ProcessBuilder("sh", "-c", command)
            .redirectErrorStream(true)
            .directory(File("/"))
            .start()
        val out = StringBuilder()
        val reader = Thread {
            proc.inputStream.bufferedReader().use { r -> r.forEachLine { out.append(it).append('\n') } }
        }
        reader.start()
        val finished = proc.waitFor(timeoutSeconds.toLong().coerceAtLeast(1), TimeUnit.SECONDS)
        if (!finished) {
            proc.destroyForcibly()
            reader.join(1000)
            return out.toString() + "\n__SAPU_TIMEOUT__"
        }
        reader.join(5000)
        return out.toString() + "\n__SAPU_EXIT__=" + proc.exitValue()
    }

    override fun destroy() {
        exitProcess(0)
    }
}
