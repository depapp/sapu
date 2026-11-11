package id.sapu.app.core

import android.content.Context
import android.content.pm.ApplicationInfo
import android.content.pm.PackageManager
import id.sapu.app.rules.Bloat
import id.sapu.app.rules.Rules

data class AppRow(
    val pkg: String,
    val name: String,
    val system: Boolean,
    val installed: Boolean,
    val protected: Boolean,
    val bloat: Bloat?,
    val dataProtected: Boolean,
    val size: SizeEntry? = null,
    val usage: Usage? = null,
)

/**
 * Package inventory from PackageManager. Works without Shizuku; removing or
 * restoring anything still needs the funnel.
 */
class Apps(private val context: Context, private val rules: Rules) {
    private val pm = context.packageManager

    fun list(): List<AppRow> {
        val all = pm.getInstalledApplications(PackageManager.MATCH_UNINSTALLED_PACKAGES)
        return all.filter { it.packageName != context.packageName }.map { ai ->
            val installed = ai.flags and ApplicationInfo.FLAG_INSTALLED != 0
            AppRow(
                pkg = ai.packageName,
                name = label(ai),
                system = ai.flags and ApplicationInfo.FLAG_SYSTEM != 0,
                installed = installed,
                protected = rules.isProtected(ai.packageName),
                bloat = rules.bloatFor(ai.packageName),
                dataProtected = rules.isDataProtected(ai.packageName),
            )
        }.sortedWith(compareBy({ !it.installed }, { it.name.lowercase() }))
    }

    fun installedSet(): Set<String> =
        pm.getInstalledApplications(0).mapTo(HashSet()) { it.packageName }

    private fun label(ai: ApplicationInfo): String {
        rules.nameFor(ai.packageName)?.let { return it }
        val l = try { pm.getApplicationLabel(ai).toString() } catch (e: Exception) { "" }
        return if (l.isBlank() || l == ai.packageName) ai.packageName.substringAfterLast('.') else l
    }
}
