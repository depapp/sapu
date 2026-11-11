package id.sapu.app.core

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class StatsTest {
    @Test
    fun diskstatsParsesParallelArrays() {
        val out = "Latency: 12ms\nPhotos Size: 100\nPackage Names: [\"com.a\",\"com.b\"]\nApp Sizes: [10,20]\nApp Data Sizes: [30,40]\nCache Sizes: [5]\n"
        val s = parseDiskstats(out)!!
        assertEquals(SizeEntry(10, 30, 5), s["com.a"])
        assertEquals(SizeEntry(20, 40, 0), s["com.b"])
        assertEquals(45L, s["com.a"]!!.total)
    }

    @Test
    fun diskstatsWithoutPackagesIsNull() {
        assertNull(parseDiskstats("Latency: 12ms\nPhotos Size: 100\n"))
    }

    @Test
    fun usageKeepsLatestTimeAndMostLaunches() {
        val out = "com.android.chrome|2026-11-01 10:00:00|7\ncom.android.chrome|2026-11-05 09:30:00|3\ncom.never.used|1970-01-01 00:00:00|0\ncom.old|1699999999999|0\n"
        val u = parseUsage(out)
        assertEquals(7, u["com.android.chrome"]!!.launchCount)
        assertTrue(u["com.android.chrome"]!!.lastUsedMs > 0)
        assertEquals(0L, u["com.never.used"]!!.lastUsedMs)
        assertEquals(1699999999999L, u["com.old"]!!.lastUsedMs)
    }

    @Test
    fun whitelistOnlyUserRows() {
        val out = "system-excidle,com.android.vending,10134\nuser,com.example.music,10200\nuser,,1\n"
        assertEquals(listOf("com.example.music"), parseDeviceIdleWhitelist(out))
    }
}
