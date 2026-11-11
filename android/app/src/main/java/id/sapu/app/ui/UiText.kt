package id.sapu.app.ui

import androidx.compose.runtime.Composable
import androidx.compose.ui.res.stringResource
import id.sapu.app.R

/**
 * The core (Scanner, Funnel, Safety) speaks English like the CLI and the history log.
 * These helpers translate its fixed phrases for the screen; unknown text (pm output) is shown as is.
 */
private val RUNNING = Regex("""^(\S+) is running$""")
private val PERSONAL_MEDIA = Regex("""^personal media of (\S+)$""")
private val OUTSIDE = Regex("""^path is outside shared storage: (.+)$""")
private val ROOT = Regex("""^refusing to delete a storage root: (.+)$""")

@Composable
fun uiSection(s: String): String = if (s == "Leftovers") stringResource(R.string.section_leftovers) else s

@Composable
fun uiNote(s: String): String = if (s == "personal media") stringResource(R.string.note_personal_media) else s

@Composable
fun uiRisk(risk: String): String = when (risk) {
    "safe" -> stringResource(R.string.risk_safe)
    "medium" -> stringResource(R.string.risk_medium)
    else -> risk
}

@Composable
fun uiReason(r: String): String {
    if (r.isEmpty()) return r
    RUNNING.find(r)?.let { return stringResource(R.string.reason_running, it.groupValues[1]) }
    PERSONAL_MEDIA.find(r)?.let { return stringResource(R.string.reason_personal_media, it.groupValues[1]) }
    OUTSIDE.find(r)?.let { return stringResource(R.string.reason_outside, it.groupValues[1]) }
    ROOT.find(r)?.let { return stringResource(R.string.reason_root, it.groupValues[1]) }
    val id = when (r) {
        "not readable by shell" -> R.string.reason_not_readable
        "app is installed, not a leftover" -> R.string.reason_installed
        "timed out" -> R.string.reason_timed_out
        "move failed" -> R.string.reason_move_failed
        "invalid package name" -> R.string.reason_invalid_package
        "protected package" -> R.string.reason_protected
        "pm failed" -> R.string.reason_pm_failed
        "restore failed" -> R.string.reason_restore_failed
        "compile failed" -> R.string.reason_compile_failed
        "failed" -> R.string.reason_failed
        "invalid" -> R.string.reason_invalid
        "not inside the run's trash" -> R.string.reason_not_in_trash
        "missing in trash" -> R.string.reason_missing_in_trash
        "destination exists or move failed" -> R.string.reason_destination_exists
        "path must be absolute" -> R.string.reason_not_absolute
        "path contains control characters" -> R.string.reason_control_chars
        "path contains '..'" -> R.string.reason_dotdot
        "refusing to touch Sapu's own trash through the funnel" -> R.string.reason_own_trash
        else -> return r
    }
    return stringResource(id)
}
