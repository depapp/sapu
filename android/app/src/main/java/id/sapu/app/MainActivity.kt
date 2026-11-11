package id.sapu.app

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.viewModels
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.CleaningServices
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Home
import androidx.compose.material.icons.filled.Apps
import androidx.compose.material.icons.filled.Speed
import androidx.compose.material3.CenterAlignedTopAppBar
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController
import id.sapu.app.ui.CleanScreen
import id.sapu.app.ui.DebloatScreen
import id.sapu.app.ui.HomeScreen
import id.sapu.app.ui.OptimizeScreen
import id.sapu.app.ui.SapuTheme
import id.sapu.app.ui.SapuViewModel
import id.sapu.app.ui.TrashScreen
import id.sapu.app.ui.collectAsStateLifecycle

private enum class Dest(val route: String, val label: Int, val icon: ImageVector) {
    HOME("home", R.string.nav_home, Icons.Filled.Home),
    CLEAN("clean", R.string.nav_clean, Icons.Filled.CleaningServices),
    APPS("apps", R.string.nav_apps, Icons.Filled.Apps),
    OPTIMIZE("optimize", R.string.nav_optimize, Icons.Filled.Speed),
    TRASH("trash", R.string.nav_trash, Icons.Filled.Delete),
}

class MainActivity : ComponentActivity() {
    private val vm: SapuViewModel by viewModels()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContent { SapuTheme { SapuApp(vm) } }
    }

    override fun onResume() {
        super.onResume()
        vm.refreshHome()
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun SapuApp(vm: SapuViewModel) {
    val nav = rememberNavController()
    val state by vm.state.collectAsStateLifecycle()
    val snack = remember { SnackbarHostState() }
    LaunchedEffect(state.message) {
        state.message?.let { snack.showSnackbar(it); vm.dismissMessage() }
    }
    val current by nav.currentBackStackEntryAsState()
    Scaffold(
        topBar = {
            Column {
                CenterAlignedTopAppBar(title = { Text(state.busy ?: stringResource(R.string.app_name)) })
                if (state.busy != null) LinearProgressIndicator(Modifier.fillMaxWidth())
            }
        },
        snackbarHost = { SnackbarHost(snack) },
        bottomBar = {
            NavigationBar {
                Dest.entries.forEach { d ->
                    NavigationBarItem(
                        selected = current?.destination?.route == d.route,
                        onClick = { nav.navigate(d.route) { popUpTo(Dest.HOME.route) { saveState = true }; launchSingleTop = true; restoreState = true } },
                        icon = { Icon(d.icon, stringResource(d.label)) },
                        label = { Text(stringResource(d.label)) },
                    )
                }
            }
        },
    ) { pad ->
        Box(Modifier.padding(pad)) {
            NavHost(nav, startDestination = Dest.HOME.route) {
                composable(Dest.HOME.route) { HomeScreen(vm) }
                composable(Dest.CLEAN.route) { CleanScreen(vm) }
                composable(Dest.APPS.route) { DebloatScreen(vm) }
                composable(Dest.OPTIMIZE.route) { OptimizeScreen(vm) }
                composable(Dest.TRASH.route) { TrashScreen(vm) }
            }
        }
    }
}
