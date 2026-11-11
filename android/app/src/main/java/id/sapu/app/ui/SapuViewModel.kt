package id.sapu.app.ui

import android.app.Application
import android.os.Environment
import android.os.StatFs
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import id.sapu.app.R
import id.sapu.app.core.ActionResult
import id.sapu.app.core.AppRow
import id.sapu.app.core.Apps
import id.sapu.app.core.Candidate
import id.sapu.app.core.Funnel
import id.sapu.app.core.History
import id.sapu.app.core.HistoryEntry
import id.sapu.app.core.Scanner
import id.sapu.app.core.SizeEntry
import id.sapu.app.core.Stats
import id.sapu.app.core.Usage
import id.sapu.app.core.TrashRun
import id.sapu.app.core.TrashStore
import id.sapu.app.rules.Rules
import id.sapu.app.shell.ShizukuBridge
import id.sapu.app.shell.ShizukuStatus
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

data class Storage(val total: Long, val free: Long) {
    val used get() = total - free
    val percent get() = if (total == 0L) 0 else (used * 100 / total).toInt()
}

data class UiState(
    val shizuku: ShizukuStatus = ShizukuStatus.NOT_RUNNING,
    val storage: Storage = Storage(0, 0),
    val apps: List<AppRow> = emptyList(),
    val candidates: List<Candidate> = emptyList(),
    val selected: Set<String> = emptySet(),
    val scanned: Boolean = false,
    val lastResults: List<ActionResult> = emptyList(),
    val lastCommand: String = "",
    val trash: List<TrashRun> = emptyList(),
    val history: List<HistoryEntry> = emptyList(),
    val statsLoaded: Boolean = false,
    val batteryExempt: List<String> = emptyList(),
    val busy: String? = null,
    val message: String? = null,
)

class SapuViewModel(app: Application) : AndroidViewModel(app) {
    private val rules = Rules.load(app)
    private val history = History(app)
    private val apps = Apps(app, rules)
    private val shell = ShizukuBridge

    private fun str(id: Int, vararg args: Any) = getApplication<Application>().getString(id, *args)

    private var sizes: Map<String, SizeEntry> = emptyMap()
    private var usage: Map<String, Usage> = emptyMap()

    private val _state = MutableStateFlow(UiState())
    val state: StateFlow<UiState> = _state

    init {
        ShizukuBridge.init(app)
        viewModelScope.launch { ShizukuBridge.status.collect { s -> _state.update { it.copy(shizuku = s) } } }
        refreshHome()
    }

    val ready get() = _state.value.shizuku == ShizukuStatus.READY

    fun refreshHome() {
        ShizukuBridge.refresh()
        viewModelScope.launch {
            val st = withContext(Dispatchers.IO) {
                val fs = StatFs(Environment.getDataDirectory().path)
                Storage(fs.totalBytes, fs.availableBytes)
            }
            val list = withContext(Dispatchers.IO) { apps.list() }
            val hist = history.read(50)
            _state.update { it.copy(storage = st, apps = enrich(list), history = hist) }
        }
    }

    private fun enrich(rows: List<AppRow>) = rows.map { it.copy(size = sizes[it.pkg], usage = usage[it.pkg]) }

    /** Per-app sizes, launch counts, last-used dates and battery exemptions. Read-only. */
    fun loadStats() = guarded(str(R.string.busy_measuring)) {
        val st = Stats(shell)
        sizes = st.sizes(refresh = _state.value.statsLoaded)
        usage = st.usage()
        val exempt = st.batteryExempt()
        _state.update { it.copy(apps = enrich(it.apps), batteryExempt = exempt, statsLoaded = true) }
    }

    /** Compiles the given apps with speed-profile, then runs bg-dexopt. Removes nothing. */
    fun optimize(pkgs: List<String>, dryRun: Boolean) = guarded(str(if (dryRun) R.string.busy_previewing else R.string.busy_optimizing)) {
        val f = Funnel(shell, rules, history, "optimize", dryRun)
        val results = ArrayList<ActionResult>()
        for (p in pkgs) results.add(f.compile(p))
        results.add(f.dexopt())
        val compiled = results.count { it.action == "COMPILED" || it.action == "WOULD-COMPILE" }
        val dex = results.last()
        val message = when {
            dryRun -> str(R.string.msg_optimize_preview, compiled)
            dex.action == "DEXOPTED" -> str(R.string.msg_optimized, compiled)
            else -> str(R.string.msg_optimized_no_dexopt, compiled, dex.reason.ifEmpty { dex.action })
        }
        _state.update { it.copy(lastResults = results, lastCommand = "optimize", message = message) }
    }

    fun dismissMessage() = _state.update { it.copy(message = null) }

    fun requestShizuku() = ShizukuBridge.requestPermission()

    fun scan() = guarded(str(R.string.busy_scanning)) {
        val installed = withContext(Dispatchers.IO) { apps.installedSet() }
        val found = Scanner(shell, rules).scan(installed)
        _state.update {
            it.copy(
                candidates = found,
                selected = found.filter { c -> c.skip.isEmpty() }.map { c -> c.path }.toSet(),
                scanned = true,
                lastResults = emptyList(),
            )
        }
    }

    fun toggle(path: String) = _state.update {
        val s = it.selected.toMutableSet()
        if (!s.remove(path)) s.add(path)
        it.copy(selected = s)
    }

    fun clean(dryRun: Boolean, trim: Boolean) = guarded(str(if (dryRun) R.string.busy_previewing else R.string.busy_sweeping)) {
        val installed = withContext(Dispatchers.IO) { apps.installedSet() }
        val f = Funnel(shell, rules, history, "clean", dryRun, installed)
        val results = ArrayList<ActionResult>()
        val chosen = _state.value.candidates.filter { it.path in _state.value.selected && it.skip.isEmpty() }
        for (c in chosen) results.add(f.trash(c.path, c.bytes))
        if (trim) results.add(f.trimCaches())
        val freed = results.filter { !it.skipped }.sumOf { it.bytes }
        _state.update {
            it.copy(
                lastResults = results,
                lastCommand = "clean",
                message = if (dryRun) str(R.string.msg_clean_preview, results.count { r -> !r.skipped }, humanBytes(freed))
                else str(R.string.msg_swept, humanBytes(freed), f.runId),
            )
        }
        if (!dryRun) {
            scanSilently()
            refreshHome()
        }
    }

    private suspend fun scanSilently() {
        val installed = withContext(Dispatchers.IO) { apps.installedSet() }
        val found = Scanner(shell, rules).scan(installed)
        _state.update { it.copy(candidates = found, selected = found.filter { c -> c.skip.isEmpty() }.map { c -> c.path }.toSet()) }
    }

    fun removeApps(pkgs: Collection<String>, dryRun: Boolean) = guarded(str(if (dryRun) R.string.busy_previewing else R.string.busy_removing)) {
        val f = Funnel(shell, rules, history, "uninstall", dryRun)
        val results = pkgs.map { f.uninstall(it) }
        val done = results.count { !it.skipped }
        _state.update { it.copy(lastResults = results, lastCommand = "apps", message = str(if (dryRun) R.string.msg_would_remove else R.string.msg_removed, done)) }
        if (!dryRun) refreshHome()
    }

    fun restoreApps(pkgs: Collection<String>) = guarded(str(R.string.busy_restoring_apps)) {
        val f = Funnel(shell, rules, history, "restore", false)
        val results = pkgs.map { f.restore(it) }
        _state.update { it.copy(lastResults = results, lastCommand = "apps", message = str(R.string.msg_restored_apps, results.count { r -> !r.skipped })) }
        refreshHome()
    }

    fun loadTrash() = guarded(str(R.string.busy_reading_trash)) {
        _state.update { it.copy(trash = TrashStore(shell).runs()) }
    }

    fun restoreRun(run: TrashRun) = guarded(str(R.string.busy_restoring, run.id)) {
        val f = Funnel(shell, rules, history, "restore", false)
        val results = run.items.map { f.restoreFile(run.dir, it.from, it.origin) }
        val ok = results.count { !it.skipped }
        if (ok == run.items.size) TrashStore(shell).empty(run)
        _state.update { it.copy(message = str(R.string.msg_restored_items, ok, run.items.size), trash = TrashStore(shell).runs()) }
        refreshHome()
    }

    fun emptyRun(run: TrashRun) = guarded(str(R.string.busy_deleting, run.id)) {
        val store = TrashStore(shell)
        val ok = store.empty(run)
        _state.update { it.copy(message = str(if (ok) R.string.msg_deleted else R.string.msg_delete_failed, run.id), trash = store.runs()) }
        refreshHome()
    }

    private fun guarded(label: String, block: suspend () -> Unit) {
        if (_state.value.busy != null) return
        if (!ready) {
            _state.update { it.copy(message = str(R.string.msg_need_shizuku)) }
            return
        }
        viewModelScope.launch {
            _state.update { it.copy(busy = label) }
            try {
                block()
            } catch (e: Exception) {
                _state.update { it.copy(message = e.message ?: e.javaClass.simpleName) }
            } finally {
                _state.update { it.copy(busy = null) }
            }
        }
    }
}
