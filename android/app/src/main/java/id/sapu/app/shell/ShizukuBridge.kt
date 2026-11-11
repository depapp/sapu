package id.sapu.app.shell

import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.ServiceConnection
import android.content.pm.PackageManager
import android.net.Uri
import android.os.IBinder
import id.sapu.app.BuildConfig
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeout
import rikka.shizuku.Shizuku

/** Where the app stands with Shizuku. Drives what the UI can offer. */
enum class ShizukuStatus {
    NOT_INSTALLED,
    NOT_RUNNING,
    PERMISSION_NEEDED,
    READY,
}

const val SHIZUKU_PACKAGE = "moe.shizuku.privileged.api"
private const val PERMISSION_REQUEST = 7310

/**
 * Owns the Shizuku binder, the permission handshake and the bound
 * [ShellService]. Everything privileged in Sapu goes through [run].
 */
object ShizukuBridge : Shell {
    private val _status = MutableStateFlow(ShizukuStatus.NOT_RUNNING)
    val status: StateFlow<ShizukuStatus> = _status

    private var service: IShellService? = null
    private var pending: CompletableDeferred<IShellService>? = null
    private var appContext: Context? = null

    private val userServiceArgs by lazy {
        Shizuku.UserServiceArgs(ComponentName(BuildConfig.APPLICATION_ID, ShellService::class.java.name))
            .daemon(false)
            .processNameSuffix("shell")
            .debuggable(BuildConfig.DEBUG)
            .version(BuildConfig.VERSION_CODE)
    }

    private val connection = object : ServiceConnection {
        override fun onServiceConnected(name: ComponentName, binder: IBinder) {
            val s = IShellService.Stub.asInterface(binder)
            service = s
            pending?.complete(s)
            pending = null
        }

        override fun onServiceDisconnected(name: ComponentName) {
            service = null
        }
    }

    fun init(context: Context) {
        if (appContext != null) return
        appContext = context.applicationContext
        Shizuku.addBinderReceivedListenerSticky { refresh() }
        Shizuku.addBinderDeadListener {
            service = null
            refresh()
        }
        Shizuku.addRequestPermissionResultListener { code, result ->
            if (code == PERMISSION_REQUEST) refresh()
        }
        refresh()
    }

    fun refresh() {
        val ctx = appContext ?: return
        _status.value = when {
            !isInstalled(ctx) -> ShizukuStatus.NOT_INSTALLED
            !Shizuku.pingBinder() -> ShizukuStatus.NOT_RUNNING
            Shizuku.checkSelfPermission() != PackageManager.PERMISSION_GRANTED -> ShizukuStatus.PERMISSION_NEEDED
            else -> ShizukuStatus.READY
        }
    }

    private fun isInstalled(ctx: Context): Boolean = try {
        ctx.packageManager.getPackageInfo(SHIZUKU_PACKAGE, 0)
        true
    } catch (e: PackageManager.NameNotFoundException) {
        false
    }

    fun requestPermission() {
        if (Shizuku.pingBinder() && Shizuku.checkSelfPermission() != PackageManager.PERMISSION_GRANTED) {
            Shizuku.requestPermission(PERMISSION_REQUEST)
        }
    }

    /** Opens Shizuku itself, or its download page when it is not installed. */
    fun openShizuku(ctx: Context) {
        val pm = ctx.packageManager
        val launch = pm.getLaunchIntentForPackage(SHIZUKU_PACKAGE)
        val intent = launch ?: Intent(Intent.ACTION_VIEW, Uri.parse("https://shizuku.rikka.app/download/"))
        intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        ctx.startActivity(intent)
    }

    private suspend fun bound(): IShellService {
        service?.let { return it }
        pending?.let { return it.await() }
        val d = CompletableDeferred<IShellService>()
        pending = d
        Shizuku.bindUserService(userServiceArgs, connection)
        return withTimeout(20_000) { d.await() }
    }

    override suspend fun run(command: String, timeoutSeconds: Int): ShellResult = withContext(Dispatchers.IO) {
        check(_status.value == ShizukuStatus.READY) { "Shizuku is not ready" }
        val raw = bound().run(command, timeoutSeconds)
        parseShellOutput(raw)
    }
}
