plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
    id("org.jetbrains.kotlin.plugin.compose")
}

android {
    namespace = "id.sapu.app"
    compileSdk = 34

    defaultConfig {
        applicationId = "id.sapu.app"
        minSdk = 26
        targetSdk = 34
        versionCode = 2
        versionName = "0.2.0"
    }

    // Release signing comes from the environment (CI secrets or a local keystore).
    // Without SAPU_KEYSTORE_FILE the release build is unsigned, so anyone can still build it.
    val keystore = System.getenv("SAPU_KEYSTORE_FILE")?.takeIf { it.isNotBlank() }?.let { file(it) }
    signingConfigs {
        create("release") {
            if (keystore != null) {
                storeFile = keystore
                storePassword = System.getenv("SAPU_KEYSTORE_PASSWORD")
                keyAlias = System.getenv("SAPU_KEY_ALIAS")
                keyPassword = System.getenv("SAPU_KEY_PASSWORD")
            }
        }
    }
    buildTypes {
        release {
            isMinifyEnabled = false
            if (keystore != null) signingConfig = signingConfigs.getByName("release")
        }
    }
    buildFeatures {
        compose = true
        aidl = true
        buildConfig = true
    }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions { jvmTarget = "17" }
    // The rules database is shared with the CLI: internal/rules/data is the single source of truth.
    sourceSets["main"].assets.srcDir(rootProject.layout.projectDirectory.dir("../internal/rules/data"))
}

dependencies {
    val composeBom = platform("androidx.compose:compose-bom:2024.09.03")
    implementation(composeBom)
    implementation("androidx.compose.ui:ui")
    implementation("androidx.compose.material3:material3")
    implementation("androidx.compose.material:material-icons-extended")
    implementation("androidx.compose.ui:ui-tooling-preview")
    implementation("androidx.activity:activity-compose:1.9.2")
    implementation("androidx.lifecycle:lifecycle-viewmodel-compose:2.8.6")
    implementation("androidx.lifecycle:lifecycle-runtime-compose:2.8.6")
    implementation("androidx.navigation:navigation-compose:2.8.2")
    implementation("androidx.core:core-ktx:1.13.1")
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.8.1")
    implementation("dev.rikka.shizuku:api:13.1.5")
    implementation("dev.rikka.shizuku:provider:13.1.5")
    implementation("org.yaml:snakeyaml:2.2")
    testImplementation("junit:junit:4.13.2")
}
