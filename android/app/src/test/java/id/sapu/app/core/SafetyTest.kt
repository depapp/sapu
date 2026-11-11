package id.sapu.app.core

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class SafetyTest {
    @Test fun normalizesStorageSpellings() {
        assertEquals("/sdcard/Download/x.apk", Safety.normalize("/storage/emulated/0/Download/x.apk").getOrThrow())
        assertEquals("/sdcard/Download/x.apk", Safety.normalize("/sdcard/./Download//x.apk").getOrThrow())
        assertTrue(Safety.normalize("/data/user/0/com.x").isFailure)
    }

    @Test fun refusesRootsTrashAndTraversal() {
        for (p in listOf("/sdcard", "/sdcard/DCIM", "/sdcard/Android/data", "/sdcard/.sapu/trash/x", "/sdcard/Download/../DCIM", "/sdcard/a\u0000b")) {
            assertTrue(p, Safety.validate(p).isFailure)
        }
        assertEquals("/sdcard/Download/old.apk", Safety.validate("/sdcard/Download/old.apk").getOrThrow())
    }

    @Test fun leftoverOwner() {
        assertEquals("com.x", Safety.leftoverOwner("/sdcard/Android/data/com.x/cache"))
        assertEquals("", Safety.leftoverOwner("/sdcard/Download/a"))
    }

    @Test fun originRoundTrip() {
        val n = "/sdcard/Download/sub/x.apk"
        assertEquals("Download__sub__x.apk", Safety.encodeOrigin(n))
        assertEquals(n, Safety.decodeOrigin(Safety.encodeOrigin(n)))
    }

    @Test fun packageNames() {
        assertTrue(Safety.validPackage("com.whatsapp"))
        assertFalse(Safety.validPackage("com.x; rm -rf /"))
        assertFalse(Safety.validPackage("nodots"))
    }
}
