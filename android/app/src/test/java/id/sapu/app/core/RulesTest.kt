package id.sapu.app.core

import id.sapu.app.rules.Rules
import java.util.Locale
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

class RulesTest {
    private val data = File("../../internal/rules/data")

    private fun load(): Rules = Rules.parse(
        protected = File(data, "protected_packages.yaml").readText(),
        dataProtected = File(data, "data_protected.yaml").readText(),
        bloat = File(data, "bloatware.yaml").readText(),
        leftovers = File(data, "leftovers.yaml").readText(),
        cleanable = File(data, "cleanable.yaml").readText(),
        names = File(data, "app_names.yaml").readText(),
    )

    @Test fun readsTheSharedDatabase() {
        val r = load()
        assertTrue(r.isProtected("com.android.settings"))
        assertTrue(r.isProtected("com.google.android.gms"))
        assertTrue(r.isDataProtected("com.whatsapp"))
        assertFalse(r.isProtected("com.google.android.youtube"))
        assertEquals("YouTube", r.nameFor("com.google.android.youtube"))
        assertEquals("google", r.bloatFor("com.google.android.youtube")?.vendor)
        assertTrue(r.cleanable.any { it.glob == "Download/*.apk" && it.minAgeDays == 7 })
        assertTrue(r.leftovers.any { it.path == "WhatsApp" && it.pkg == "com.whatsapp" })
    }

    @Test fun noBloatEntryIsProtected() {
        val r = load()
        val bad = r.bloat.filter { r.isProtected(it.pkg) }
        assertTrue("bloat entries that are also protected: $bad", bad.isEmpty())
    }

    @Test fun indonesianTextComesFromTheSharedDatabase() {
        val r = load()
        val yt = r.bloatFor("com.google.android.youtube")!!
        assertEquals("YouTube; reinstallable from Play", yt.note)
        assertEquals("YouTube; bisa dipasang lagi dari Play", yt.noteId)
        assertTrue("every cleanable rule needs name_id", r.cleanable.all { it.nameId.isNotEmpty() })
        val before = Locale.getDefault()
        try {
            Locale.setDefault(Locale.ENGLISH)
            assertEquals(yt.note, yt.displayNote)
            assertEquals("Installer APKs", r.cleanable.first { it.glob == "Download/*.apk" }.displayName)
            Locale.setDefault(Locale("in", "ID"))
            assertEquals(yt.noteId, yt.displayNote)
            assertEquals("APK pemasang", r.cleanable.first { it.glob == "Download/*.apk" }.displayName)
            // no note_id: fall back to the English note (here a plain app name)
            assertEquals("Spotify", r.bloatFor("com.spotify.music")?.displayNote)
        } finally {
            Locale.setDefault(before)
        }
    }
}
