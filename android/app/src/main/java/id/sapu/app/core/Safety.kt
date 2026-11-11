package id.sapu.app.core

/**
 * Pure path rules shared by every destructive action. Mirrors
 * internal/safety in the CLI so both products refuse the same things.
 */
object Safety {
    const val TRASH_ROOT = "/sdcard/.sapu/trash"

    private val storageRoots = listOf("/sdcard", "/storage/emulated/0", "/storage/self/primary", "/mnt/sdcard")

    private val bareRoots = setOf(
        "", "Android", "Android/data", "Android/obb", "Android/media", "DCIM", "DCIM/Camera", "Pictures",
        "Movies", "Music", "Download", "Documents", "Alarms", "Audiobooks", "Notifications", "Podcasts",
        "Recordings", "Ringtones", ".sapu", ".sapu/trash",
    )

    private val packageRe = Regex("^[A-Za-z][A-Za-z0-9_]*(\\.[A-Za-z0-9_]+)+$")

    fun validPackage(pkg: String) = packageRe.matches(pkg)

    /** Maps any spelling of shared storage onto /sdcard, or fails. */
    fun normalize(p0: String): Result<String> {
        val p = p0.trim()
        if (p.isEmpty() || !p.startsWith("/")) return Result.failure(IllegalArgumentException("path must be absolute"))
        for (r in storageRoots) {
            if (p == r || p.startsWith("$r/")) {
                return Result.success(clean("/sdcard" + p.removePrefix(r)))
            }
        }
        return Result.failure(IllegalArgumentException("path is outside shared storage: $p"))
    }

    /** The deny gate. A path that fails here is never touched. */
    fun validate(p: String): Result<String> {
        if (p.any { it.code < 0x20 || it.code == 0x7f }) {
            return Result.failure(IllegalArgumentException("path contains control characters"))
        }
        if (p.split("/").any { it == ".." }) {
            return Result.failure(IllegalArgumentException("path contains '..'"))
        }
        val n = normalize(p).getOrElse { return Result.failure(it) }
        val rel = n.removePrefix("/sdcard").removePrefix("/")
        if (rel in bareRoots) return Result.failure(IllegalArgumentException("refusing to delete a storage root: $n"))
        if (rel == ".sapu" || rel.startsWith(".sapu/")) {
            return Result.failure(IllegalArgumentException("refusing to touch Sapu's own trash through the funnel"))
        }
        return Result.success(n)
    }

    /** Package owning an Android/{data,obb,media}/<pkg> path, or "". */
    fun leftoverOwner(p: String): String {
        val rel = p.removePrefix("/sdcard/")
        for (pre in listOf("Android/data/", "Android/obb/", "Android/media/")) {
            if (rel.startsWith(pre)) return rel.removePrefix(pre).substringBefore("/")
        }
        return ""
    }

    fun encodeOrigin(n: String) = n.removePrefix("/sdcard/").replace("/", "__")
    fun decodeOrigin(name: String) = "/sdcard/" + name.replace("__", "/")

    private fun clean(p: String): String {
        val parts = ArrayList<String>()
        for (seg in p.split("/")) {
            when (seg) {
                "", "." -> {}
                ".." -> if (parts.isNotEmpty()) parts.removeAt(parts.size - 1)
                else -> parts.add(seg)
            }
        }
        return "/" + parts.joinToString("/")
    }
}
