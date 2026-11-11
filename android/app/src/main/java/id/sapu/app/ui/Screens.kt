package id.sapu.app.ui

import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.AssistChip
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.Checkbox
import androidx.compose.material3.FilterChip
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import id.sapu.app.R
import id.sapu.app.core.ActionResult
import id.sapu.app.core.AppRow
import id.sapu.app.core.TrashRun
import id.sapu.app.shell.ShizukuBridge
import id.sapu.app.shell.ShizukuStatus

@Composable
private fun Section(title: String, content: @Composable () -> Unit) {
    Card(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 6.dp)) {
        Column(Modifier.padding(16.dp)) {
            Text(title, style = MaterialTheme.typography.titleMedium)
            Spacer(Modifier.height(8.dp))
            content()
        }
    }
}

@Composable
fun ShizukuCard(status: ShizukuStatus, onRequest: () -> Unit, onRefresh: () -> Unit) {
    val ctx = LocalContext.current
    val (title, body) = when (status) {
        ShizukuStatus.READY -> R.string.shizuku_ready_title to R.string.shizuku_ready_body
        ShizukuStatus.PERMISSION_NEEDED -> R.string.shizuku_perm_title to R.string.shizuku_perm_body
        ShizukuStatus.NOT_RUNNING -> R.string.shizuku_off_title to R.string.shizuku_off_body
        ShizukuStatus.NOT_INSTALLED -> R.string.shizuku_missing_title to R.string.shizuku_missing_body
    }
    Section(stringResource(title)) {
        Text(stringResource(body), style = MaterialTheme.typography.bodyMedium)
        Spacer(Modifier.height(8.dp))
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            when (status) {
                ShizukuStatus.PERMISSION_NEEDED -> Button(onClick = onRequest) { Text(stringResource(R.string.allow)) }
                ShizukuStatus.NOT_RUNNING -> Button(onClick = { ShizukuBridge.openShizuku(ctx) }) { Text(stringResource(R.string.open_shizuku)) }
                ShizukuStatus.NOT_INSTALLED -> Button(onClick = { ShizukuBridge.openShizuku(ctx) }) { Text(stringResource(R.string.get_shizuku)) }
                ShizukuStatus.READY -> {}
            }
            OutlinedButton(onClick = onRefresh) { Text(stringResource(R.string.refresh)) }
        }
    }
}

@Composable
fun HomeScreen(vm: SapuViewModel) {
    val s by vm.state.collectAsStateLifecycle()
    LazyColumn(Modifier.fillMaxSize(), contentPadding = PaddingValues(vertical = 8.dp)) {
        item {
            Section(stringResource(R.string.storage)) {
                Text(stringResource(R.string.used_of, humanBytes(s.storage.used), humanBytes(s.storage.total)), style = MaterialTheme.typography.headlineSmall)
                Spacer(Modifier.height(8.dp))
                LinearProgressIndicator(progress = { s.storage.percent / 100f }, modifier = Modifier.fillMaxWidth())
                Spacer(Modifier.height(4.dp))
                Text(stringResource(R.string.free_space, humanBytes(s.storage.free)), style = MaterialTheme.typography.bodySmall)
            }
        }
        item { ShizukuCard(s.shizuku, vm::requestShizuku, vm::refreshHome) }
        item {
            val installed = s.apps.filter { it.installed }
            Section(stringResource(R.string.apps)) {
                Text(stringResource(R.string.app_counts, installed.count { !it.system }, installed.count { it.system }))
                val bloat = installed.count { it.bloat != null && !it.protected }
                val removed = s.apps.count { !it.installed }
                Text(stringResource(R.string.bloat_counts, bloat, removed), style = MaterialTheme.typography.bodySmall)
                if (s.statsLoaded) {
                    Text(
                        stringResource(R.string.app_sizes_summary, humanBytes(installed.sumOf { it.size?.cache ?: 0L }), humanBytes(installed.sumOf { it.size?.data ?: 0L })),
                        style = MaterialTheme.typography.bodySmall,
                    )
                } else {
                    TextButton(onClick = vm::loadStats, enabled = s.busy == null && s.shizuku == ShizukuStatus.READY) { Text(stringResource(R.string.measure_sizes)) }
                }
            }
        }
        if (s.history.isNotEmpty()) {
            item {
                Section(stringResource(R.string.recent_activity)) {
                    s.history.take(8).forEach { h ->
                        Text("${h.time.replace('T', ' ')}  ${h.action}  ${h.path.removePrefix("/sdcard/")}", style = MaterialTheme.typography.bodySmall)
                    }
                }
            }
        }
        item {
            Section(stringResource(R.string.safety_title)) {
                Text(stringResource(R.string.safety_body), style = MaterialTheme.typography.bodySmall)
            }
        }
    }
}

@Composable
fun CleanScreen(vm: SapuViewModel) {
    val s by vm.state.collectAsStateLifecycle()
    var confirm by remember { mutableStateOf(false) }
    var trim by rememberSaveable { mutableStateOf(true) }
    val selectedBytes = s.candidates.filter { it.path in s.selected && it.skip.isEmpty() }.sumOf { it.bytes }
    Column(Modifier.fillMaxSize()) {
        if (s.shizuku != ShizukuStatus.READY) ShizukuCard(s.shizuku, vm::requestShizuku, vm::refreshHome)
        Row(Modifier.padding(16.dp), horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
            Button(onClick = vm::scan, enabled = s.busy == null && s.shizuku == ShizukuStatus.READY) { Text(stringResource(if (s.scanned) R.string.rescan else R.string.scan)) }
            if (s.scanned) {
                OutlinedButton(onClick = { vm.clean(dryRun = true, trim = trim) }, enabled = s.busy == null) { Text(stringResource(R.string.preview)) }
                Button(onClick = { confirm = true }, enabled = s.busy == null && (s.selected.isNotEmpty() || trim)) { Text(stringResource(R.string.sweep_bytes, humanBytes(selectedBytes))) }
            }
        }
        if (s.scanned) {
            Row(Modifier.padding(horizontal = 16.dp), verticalAlignment = Alignment.CenterVertically) {
                Checkbox(checked = trim, onCheckedChange = { trim = it })
                Column {
                    Text(stringResource(R.string.trim_title), style = MaterialTheme.typography.bodyMedium)
                    Text(stringResource(R.string.trim_body), style = MaterialTheme.typography.bodySmall)
                }
            }
        }
        LazyColumn(Modifier.fillMaxSize()) {
            if (s.lastResults.isNotEmpty() && s.lastCommand == "clean") item { ResultsCard(s.lastResults) }
            if (s.scanned && s.candidates.isEmpty()) item { Text(stringResource(R.string.nothing_to_sweep), Modifier.padding(16.dp)) }
            val sections = s.candidates.groupBy { it.section }
            sections.forEach { (section, list) ->
                item {
                    Text(
                        "${uiSection(section)}  ${humanBytes(list.sumOf { it.bytes })}",
                        Modifier.padding(start = 16.dp, top = 12.dp, bottom = 4.dp),
                        style = MaterialTheme.typography.titleSmall,
                        fontWeight = FontWeight.Bold,
                    )
                }
                items(list, key = { it.path }) { c ->
                    Row(
                        Modifier.fillMaxWidth().clickable(enabled = c.skip.isEmpty()) { vm.toggle(c.path) }.padding(horizontal = 16.dp, vertical = 4.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Checkbox(checked = c.path in s.selected && c.skip.isEmpty(), onCheckedChange = null, enabled = c.skip.isEmpty())
                        Spacer(Modifier.width(8.dp))
                        Column(Modifier.weight(1f)) {
                            Text(c.path.removePrefix("/sdcard/"), style = MaterialTheme.typography.bodyMedium)
                            val sub = listOf(c.owner, uiNote(c.note), uiReason(c.skip)).filter { it.isNotEmpty() }.joinToString(" · ")
                            if (sub.isNotEmpty()) Text(sub, style = MaterialTheme.typography.bodySmall, color = if (c.skip.isNotEmpty()) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant)
                        }
                        Text(humanBytes(c.bytes), style = MaterialTheme.typography.bodySmall)
                    }
                }
            }
        }
    }
    if (confirm) {
        AlertDialog(
            onDismissRequest = { confirm = false },
            title = { Text(stringResource(R.string.sweep_confirm_title, s.selected.size)) },
            text = { Text(stringResource(R.string.sweep_confirm_body, humanBytes(selectedBytes)) + if (trim) " " + stringResource(R.string.trim_permanent) else "") },
            confirmButton = { Button(onClick = { confirm = false; vm.clean(dryRun = false, trim = trim) }) { Text(stringResource(R.string.sweep)) } },
            dismissButton = { TextButton(onClick = { confirm = false }) { Text(stringResource(R.string.cancel)) } },
        )
    }
}

@Composable
private fun ResultsCard(results: List<ActionResult>) {
    Section(stringResource(R.string.last_run)) {
        results.take(40).forEach { r ->
            val label = r.path.removePrefix("/sdcard/")
            Text(
                "${r.action}  $label  ${if (r.bytes > 0) humanBytes(r.bytes) else ""}  ${uiReason(r.reason)}".trim(),
                style = MaterialTheme.typography.bodySmall,
                color = if (r.skipped) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurface,
            )
        }
        if (results.size > 40) Text(stringResource(R.string.and_more, results.size - 40), style = MaterialTheme.typography.bodySmall)
    }
}

private enum class AppTab(val label: Int) {
    REMOVABLE(R.string.tab_removable), USER(R.string.tab_user), ALL(R.string.tab_all), REMOVED(R.string.tab_removed)
}

@Composable
fun DebloatScreen(vm: SapuViewModel) {
    val s by vm.state.collectAsStateLifecycle()
    var tab by rememberSaveable { mutableStateOf(AppTab.REMOVABLE) }
    var picked by rememberSaveable { mutableStateOf(setOf<String>()) }
    var confirm by remember { mutableStateOf(false) }
    val filtered = when (tab) {
        AppTab.REMOVABLE -> s.apps.filter { it.installed && it.system && it.bloat != null && !it.protected }
        AppTab.USER -> s.apps.filter { it.installed && !it.system }
        AppTab.ALL -> s.apps.filter { it.installed }
        AppTab.REMOVED -> s.apps.filter { !it.installed }
    }
    val rows = if (s.statsLoaded) filtered.sortedByDescending { it.size?.total ?: 0L } else filtered
    Column(Modifier.fillMaxSize()) {
        if (s.shizuku != ShizukuStatus.READY) ShizukuCard(s.shizuku, vm::requestShizuku, vm::refreshHome)
        Row(Modifier.horizontalScroll(rememberScrollState()).padding(horizontal = 16.dp, vertical = 8.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            AppTab.entries.forEach { t ->
                FilterChip(selected = tab == t, onClick = { tab = t; picked = emptySet() }, label = { Text(stringResource(t.label)) })
            }
        }
        Row(Modifier.padding(horizontal = 16.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            if (tab == AppTab.REMOVED) {
                Button(onClick = { vm.restoreApps(picked); picked = emptySet() }, enabled = picked.isNotEmpty() && s.busy == null) { Text(stringResource(R.string.restore_n, picked.size)) }
            } else {
                OutlinedButton(onClick = { vm.removeApps(picked, dryRun = true) }, enabled = picked.isNotEmpty() && s.busy == null) { Text(stringResource(R.string.preview)) }
                Button(onClick = { confirm = true }, enabled = picked.isNotEmpty() && s.busy == null) { Text(stringResource(R.string.remove_n, picked.size)) }
            }
        }
        TextButton(onClick = vm::loadStats, enabled = s.busy == null && s.shizuku == ShizukuStatus.READY, modifier = Modifier.padding(horizontal = 8.dp)) {
            Text(stringResource(if (s.statsLoaded) R.string.measured_hint else R.string.measure_usage))
        }
        LazyColumn(Modifier.fillMaxSize()) {
            if (s.lastResults.isNotEmpty() && s.lastCommand == "apps") item { ResultsCard(s.lastResults) }
            if (rows.isEmpty()) item { Text(stringResource(if (tab == AppTab.REMOVED) R.string.nothing_removed else R.string.no_apps), Modifier.padding(16.dp)) }
            items(rows, key = { it.pkg }) { a -> AppRowView(a, a.pkg in picked, tab == AppTab.REMOVED) { picked = if (a.pkg in picked) picked - a.pkg else picked + a.pkg } }
        }
    }
    if (confirm) {
        val names = s.apps.filter { it.pkg in picked }.joinToString(", ") { it.name }
        AlertDialog(
            onDismissRequest = { confirm = false },
            title = { Text(stringResource(R.string.remove_confirm_title, picked.size)) },
            text = { Text(stringResource(R.string.remove_confirm_body, names)) },
            confirmButton = { Button(onClick = { confirm = false; vm.removeApps(picked, dryRun = false); picked = emptySet() }) { Text(stringResource(R.string.remove)) } },
            dismissButton = { TextButton(onClick = { confirm = false }) { Text(stringResource(R.string.cancel)) } },
        )
    }
}

@Composable
private fun AppRowView(a: AppRow, checked: Boolean, removedList: Boolean, onToggle: () -> Unit) {
    val selectable = removedList || !a.protected
    Row(
        Modifier.fillMaxWidth().clickable(enabled = selectable) { onToggle() }.padding(horizontal = 16.dp, vertical = 6.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Checkbox(checked = checked, onCheckedChange = null, enabled = selectable)
        Spacer(Modifier.width(8.dp))
        Column(Modifier.weight(1f)) {
            Text(a.name, style = MaterialTheme.typography.bodyLarge)
            Text(a.pkg, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            a.bloat?.displayNote?.takeIf { it.isNotEmpty() }?.let { Text(it, style = MaterialTheme.typography.bodySmall) }
            val u = a.usage
            val meta = listOfNotNull(
                when {
                    u == null -> null
                    u.lastUsedMs > 0 -> stringResource(R.string.opened_ago, ago(u.lastUsedMs))
                    a.installed -> stringResource(R.string.never_opened)
                    else -> null
                },
                a.size?.cache?.takeIf { it > 0 }?.let { stringResource(R.string.cache_size, humanBytes(it)) },
            ).joinToString(" · ")
            if (meta.isNotEmpty()) Text(meta, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        Column(horizontalAlignment = Alignment.End) {
            when {
                a.protected -> AssistChip(onClick = {}, label = { Text(stringResource(R.string.protected_chip)) })
                a.dataProtected -> AssistChip(onClick = {}, label = { Text(stringResource(R.string.has_your_data)) })
                a.bloat != null -> AssistChip(onClick = {}, label = { Text(uiRisk(a.bloat.risk)) })
            }
            a.size?.let { Text(humanBytes(it.total), style = MaterialTheme.typography.bodySmall) }
        }
    }
}

private const val DAY_MS = 86_400_000L
private const val UNUSED_DAYS = 90
private const val COMPILE_TOP = 5

/** Relative age of a timestamp: today, N days, N months or N years ago. */
@Composable
fun ago(ms: Long, now: Long = System.currentTimeMillis()): String {
    val d = (now - ms) / DAY_MS
    return when {
        d < 1 -> stringResource(R.string.today)
        d < 30 -> stringResource(R.string.days_ago, d)
        d < 365 -> stringResource(R.string.months_ago, d / 30)
        else -> stringResource(R.string.years_ago, d / 365)
    }
}

@Composable
fun OptimizeScreen(vm: SapuViewModel) {
    val s by vm.state.collectAsStateLifecycle()
    var confirm by remember { mutableStateOf(false) }
    val now = System.currentTimeMillis()
    val installed = s.apps.filter { it.installed }
    val top = installed.filter { (it.usage?.launchCount ?: 0) > 0 }.sortedByDescending { it.usage?.launchCount ?: 0 }.take(COMPILE_TOP)
    val unused = installed.filter { !it.system && (it.usage?.lastUsedMs ?: 0L) > 0L && now - (it.usage?.lastUsedMs ?: now) > UNUSED_DAYS * DAY_MS }
        .sortedBy { it.usage?.lastUsedMs ?: 0L }
    val byPkg = s.apps.associateBy { it.pkg }
    Column(Modifier.fillMaxSize()) {
        if (s.shizuku != ShizukuStatus.READY) ShizukuCard(s.shizuku, vm::requestShizuku, vm::refreshHome)
        Row(Modifier.padding(16.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Button(onClick = vm::loadStats, enabled = s.busy == null && s.shizuku == ShizukuStatus.READY) { Text(stringResource(if (s.statsLoaded) R.string.reanalyze else R.string.analyze)) }
            if (s.statsLoaded) {
                OutlinedButton(onClick = { vm.optimize(top.map { it.pkg }, dryRun = true) }, enabled = s.busy == null) { Text(stringResource(R.string.preview)) }
                Button(onClick = { confirm = true }, enabled = s.busy == null) { Text(stringResource(R.string.optimize)) }
            }
        }
        LazyColumn(Modifier.fillMaxSize()) {
            if (s.lastResults.isNotEmpty() && s.lastCommand == "optimize") item { ResultsCard(s.lastResults) }
            if (!s.statsLoaded) {
                item { Text(stringResource(R.string.optimize_intro), Modifier.padding(16.dp)) }
            } else {
                item {
                    Section(stringResource(R.string.battery)) {
                        if (s.batteryExempt.isEmpty()) Text(stringResource(R.string.no_battery_exempt), style = MaterialTheme.typography.bodyMedium)
                        s.batteryExempt.forEach { p ->
                            Text(stringResource(R.string.battery_exempt_row, byPkg[p]?.name ?: p), style = MaterialTheme.typography.bodySmall)
                        }
                        if (unused.isNotEmpty()) {
                            Spacer(Modifier.height(8.dp))
                            Text(stringResource(R.string.not_opened_days, UNUSED_DAYS), style = MaterialTheme.typography.bodyMedium)
                            unused.take(10).forEach { a -> Text(stringResource(R.string.unused_row, a.name, ago(a.usage?.lastUsedMs ?: 0L, now)), style = MaterialTheme.typography.bodySmall) }
                            if (unused.size > 10) Text(stringResource(R.string.and_more, unused.size - 10), style = MaterialTheme.typography.bodySmall)
                        }
                    }
                }
                item {
                    Section(stringResource(R.string.compile_title)) {
                        if (top.isEmpty()) Text(stringResource(R.string.no_launch_stats), style = MaterialTheme.typography.bodyMedium)
                        top.forEach { a -> Text(stringResource(R.string.launches, a.name, a.usage?.launchCount ?: 0), style = MaterialTheme.typography.bodySmall) }
                        Spacer(Modifier.height(8.dp))
                        Text(stringResource(R.string.dexopt_note), style = MaterialTheme.typography.bodySmall)
                    }
                }
            }
        }
    }
    if (confirm) {
        AlertDialog(
            onDismissRequest = { confirm = false },
            title = { Text(stringResource(R.string.optimize_confirm_title, top.size)) },
            text = { Text(stringResource(R.string.optimize_confirm_body)) },
            confirmButton = { Button(onClick = { confirm = false; vm.optimize(top.map { it.pkg }, dryRun = false) }) { Text(stringResource(R.string.optimize)) } },
            dismissButton = { TextButton(onClick = { confirm = false }) { Text(stringResource(R.string.cancel)) } },
        )
    }
}

@Composable
fun TrashScreen(vm: SapuViewModel) {
    val s by vm.state.collectAsStateLifecycle()
    var deleting by remember { mutableStateOf<TrashRun?>(null) }
    LaunchedEffect(s.shizuku) { if (s.shizuku == ShizukuStatus.READY) vm.loadTrash() }
    Column(Modifier.fillMaxSize()) {
        if (s.shizuku != ShizukuStatus.READY) ShizukuCard(s.shizuku, vm::requestShizuku, vm::refreshHome)
        Row(Modifier.padding(16.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            OutlinedButton(onClick = vm::loadTrash, enabled = s.busy == null && s.shizuku == ShizukuStatus.READY) { Text(stringResource(R.string.refresh)) }
            Text(stringResource(R.string.trash_summary, s.trash.size, humanBytes(s.trash.sumOf { it.bytes })), Modifier.align(Alignment.CenterVertically))
        }
        LazyColumn(Modifier.fillMaxSize()) {
            if (s.trash.isEmpty()) item { Text(stringResource(R.string.trash_empty), Modifier.padding(16.dp)) }
            items(s.trash, key = { it.id }) { run ->
                Section("${run.id}  ${humanBytes(run.bytes)}") {
                    run.items.take(6).forEach { Text(it.origin.removePrefix("/sdcard/"), style = MaterialTheme.typography.bodySmall) }
                    if (run.items.size > 6) Text(stringResource(R.string.and_more, run.items.size - 6), style = MaterialTheme.typography.bodySmall)
                    Spacer(Modifier.height(8.dp))
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        Button(onClick = { vm.restoreRun(run) }, enabled = s.busy == null && run.items.isNotEmpty()) { Text(stringResource(R.string.restore)) }
                        OutlinedButton(onClick = { deleting = run }, enabled = s.busy == null) { Text(stringResource(R.string.delete_permanently)) }
                    }
                }
            }
        }
    }
    deleting?.let { run ->
        AlertDialog(
            onDismissRequest = { deleting = null },
            title = { Text(stringResource(R.string.delete_confirm_title, run.id)) },
            text = { Text(stringResource(R.string.delete_confirm_body, humanBytes(run.bytes), run.items.size)) },
            confirmButton = { Button(onClick = { deleting = null; vm.emptyRun(run) }) { Text(stringResource(R.string.delete)) } },
            dismissButton = { TextButton(onClick = { deleting = null }) { Text(stringResource(R.string.cancel)) } },
        )
    }
}
