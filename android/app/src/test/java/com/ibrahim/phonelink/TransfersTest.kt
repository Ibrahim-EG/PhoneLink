package com.ibrahim.phonelink

import org.junit.Assert.*
import org.junit.Test

class TransfersTest {
    @Test fun confirmedAckIsSuccess() {
        val t = AckTracker(); val w = t.register("a")
        Thread { t.onAck("a", true) }.start()
        assertTrue(t.await("a", w, 2000))
    }

    @Test fun silenceIsNotSuccess() {
        val t = AckTracker(); val w = t.register("a")
        assertFalse(t.await("a", w, 100))
    }

    @Test fun negativeAckIsNotSuccess() {
        val t = AckTracker(); val w = t.register("a")
        t.onAck("a", false)
        assertFalse(t.await("a", w, 100))
    }

    @Test fun ackForOtherIdIsIgnored() {
        val t = AckTracker(); val w = t.register("a")
        t.onAck("b", true)
        assertFalse(t.await("a", w, 100))
    }

    @Test fun doneIdsExpire() {
        var now = 0L
        val d = DoneIds(1000, { now })
        assertFalse(d.has("x"))
        d.add("x"); assertTrue(d.has("x"))
        now = 999; assertTrue(d.has("x"))
        now = 1001; assertFalse(d.has("x"))
    }

    @Test fun idStableForSameContent() {
        val a = TransferId.ofStream("hello".byteInputStream())
        assertEquals(a, TransferId.ofStream("hello".byteInputStream()))
        assertNotEquals(a, TransferId.ofStream("hello!".byteInputStream()))
        assertEquals(16, a.length)
    }
}
