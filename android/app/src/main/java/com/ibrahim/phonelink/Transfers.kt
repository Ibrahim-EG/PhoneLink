package com.ibrahim.phonelink

import java.io.InputStream
import java.security.MessageDigest
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit

/** Waits for the other side's confirmation that a file was fully saved. Plain Kotlin so it can be unit tested. */
class AckTracker {
    class Wait { val latch = CountDownLatch(1); @Volatile var ok = false }
    private val waits = ConcurrentHashMap<String, Wait>()

    fun register(id: String): Wait { val w = Wait(); waits[id] = w; return w }
    fun onAck(id: String, ok: Boolean) { waits[id]?.let { it.ok = ok; it.latch.countDown() } }
    /** True only if a positive confirmation arrived within the time. */
    fun await(id: String, w: Wait, timeoutMs: Long): Boolean {
        val got = w.latch.await(timeoutMs, TimeUnit.MILLISECONDS)
        waits.remove(id)
        return got && w.ok
    }
}

/** Transfer ids that finished recently: a retry of one of them is confirmed again but not saved twice. */
class DoneIds(private val windowMs: Long = 10 * 60 * 1000L, private val clock: () -> Long = { System.currentTimeMillis() }) {
    private val seen = HashMap<String, Long>()
    @Synchronized fun has(id: String): Boolean { val t = seen[id] ?: return false; return clock() - t < windowMs }
    @Synchronized fun add(id: String) { seen[id] = clock() }
}

object TransferId {
    /** Same file content gives the same id, so a retry is recognisable. */
    fun ofStream(ins: InputStream): String {
        val md = MessageDigest.getInstance("SHA-256"); val buf = ByteArray(64 * 1024)
        while (true) { val r = ins.read(buf); if (r < 0) break; md.update(buf, 0, r) }
        return md.digest().take(8).joinToString("") { "%02x".format(it) }
    }
}
