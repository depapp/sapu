package id.sapu.app.rules

import android.content.Context
import org.yaml.snakeyaml.Yaml
import java.util.Locale

/** True when the app runs in Indonesian (Java spells the language "in", Android 13+ also "id"). */
fun indonesian(): Boolean = Locale.getDefault().language.let { it == "in" || it == "id" }

private fun pick(en: String, id: String) = if (indonesian() && id.isNotEmpty()) id else en

data class Bloat(val pkg: String, val vendor: String, val risk: String, val note: String, val noteId: String = "") {
    /** The note in the UI language; the YAML carries an optional Indonesian `note_id`. */
    val displayNote: String get() = pick(note, noteId)
}
data class Leftover(val path: String, val pkg: String)
data class Cleanable(
    val name: String,
    val glob: String,
    val kind: String,
    val minAgeDays: Int,
    val runningGuard: Boolean,
    val note: String,
    val nameId: String = "",
    val noteId: String = "",
) {
    val displayName: String get() = pick(name, nameId)
    val displayNote: String get() = pick(note, noteId)
}

private class NameList(val exact: Set<String>, val prefixes: List<String>) {
    fun matches(pkg: String) = pkg in exact || prefixes.any { pkg.startsWith(it) }
}

/**
 * The rules database. The YAML files are the same ones the CLI embeds
 * (internal/rules/data in the repo); Gradle adds that folder as assets.
 */
class Rules private constructor(
    private val protected: NameList,
    private val dataProtected: NameList,
    val bloat: List<Bloat>,
    val leftovers: List<Leftover>,
    val cleanable: List<Cleanable>,
    private val names: Map<String, String>,
) {
    private val bloatIdx = bloat.associateBy { it.pkg }

    fun isProtected(pkg: String) = protected.matches(pkg)
    fun isDataProtected(pkg: String) = dataProtected.matches(pkg)
    fun bloatFor(pkg: String): Bloat? = bloatIdx[pkg]
    fun nameFor(pkg: String): String? = names[pkg]

    companion object {
        @Volatile private var cached: Rules? = null

        fun load(context: Context): Rules = cached ?: synchronized(this) {
            cached ?: fromAssets(context).also { cached = it }
        }

        private fun fromAssets(context: Context): Rules {
            val am = context.assets
            fun text(name: String) = am.open(name).bufferedReader().use { it.readText() }
            return parse(
                protected = text("protected_packages.yaml"),
                dataProtected = text("data_protected.yaml"),
                bloat = text("bloatware.yaml"),
                leftovers = text("leftovers.yaml"),
                cleanable = text("cleanable.yaml"),
                names = text("app_names.yaml"),
            )
        }

        fun parse(
            protected: String,
            dataProtected: String,
            bloat: String,
            leftovers: String,
            cleanable: String,
            names: String,
        ): Rules {
            val yaml = Yaml()
            fun nameList(s: String): NameList {
                val m = yaml.load<Map<String, Any?>>(s) ?: emptyMap()
                return NameList(strings(m["exact"]).toSet(), strings(m["prefixes"]))
            }
            fun rows(s: String): List<Map<String, Any?>> =
                (yaml.load<List<Map<String, Any?>>>(s) ?: emptyList())
            return Rules(
                protected = nameList(protected),
                dataProtected = nameList(dataProtected),
                bloat = rows(bloat).map {
                    Bloat(it.str("package"), it.str("vendor"), it.str("risk"), it.str("note"), it.str("note_id"))
                },
                leftovers = rows(leftovers).map { Leftover(it.str("path"), it.str("package")) },
                cleanable = rows(cleanable).map {
                    Cleanable(
                        it.str("name"), it.str("glob"), it.str("kind"),
                        (it["min_age_days"] as? Number)?.toInt() ?: 0,
                        it["running_guard"] as? Boolean ?: false,
                        it.str("note"),
                        it.str("name_id"),
                        it.str("note_id"),
                    )
                },
                names = (yaml.load<Map<String, Any?>>(names) ?: emptyMap()).mapValues { it.value.toString() },
            )
        }

        private fun strings(v: Any?): List<String> = (v as? List<*>)?.map { it.toString() } ?: emptyList()
        private fun Map<String, Any?>.str(k: String) = this[k]?.toString() ?: ""
    }
}
