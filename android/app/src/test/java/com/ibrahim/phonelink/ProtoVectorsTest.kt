package com.ibrahim.phonelink

import org.junit.Assert.*
import org.junit.Test

/** Same fixed inputs and expected values as windows/vectors_test.go. */
class ProtoVectorsTest {
    private fun hex(s: String) = ByteArray(s.length / 2) { s.substring(it * 2, it * 2 + 2).toInt(16).toByte() }
    private fun hex(b: ByteArray) = b.joinToString("") { "%02x".format(it) }

    private val cliPub = "040217e617f0b6443928278f96999e69a23a4f2c152bdf6d6cdf66e5b80282d4ed194a7debcb97712d2dda3ca85aa8765a56f45fc758599652f2897c65306e5794"
    private val srvPub = "04d65a93977caa3d1b081852ff57a79e465f1660577304baead505dd3a48589cf350185e895372df6221ea3a137557e473fddb6755f05bd507c3c533fce9c91285"
    private val nc = ByteArray(16) { 0x33 }
    private val ns = ByteArray(16) { 0x44 }

    @Test fun commitAndCode() {
        assertEquals("9be3710efd77e6bb083d6c4922aac5dbce1e76c20aebde0557ee1006caa1a686", hex(Proto.commit(hex(cliPub), nc)))
        assertEquals("890468", Proto.sas(hex(cliPub), hex(srvPub), nc, ns))
    }

    @Test fun publicKeyRoundTrip() {
        assertEquals(cliPub, hex(Proto.raw65(Proto.fromRaw65(hex(cliPub)))))
    }

    @Test fun ecdhAndKey() {
        val priv = Proto.privFromScalar(ByteArray(32) { 0x11 })
        val shared = Proto.ecdh(priv, Proto.fromRaw65(hex(srvPub)))
        assertEquals("f5280218fb86ca52aa50593588e4b5ca", hex(Proto.keyBytes(shared, nc, ns)))
    }

    @Test fun framesMatchGoAndCounters() {
        val key = hex("f5280218fb86ca52aa50593588e4b5ca")
        val phone = Proto.Session(key, 0)
        val pc = Proto.Session(key, 1)
        assertEquals("000000159578d0bd15a7d0bc814ee4f79ee8f74a3b73f02482", hex(phone.frame("hello".toByteArray())))
        // a frame produced by the Go side is accepted by the phone session
        val goFrame = hex("0000001547ba9797777842ddc301ce22c1379d61eb6dab3fa5")
        assertEquals("world", String(phone.open(goFrame.copyOfRange(4, goFrame.size))))
        // replay of the same frame must fail
        try { phone.open(goFrame.copyOfRange(4, goFrame.size)); fail("replay accepted") } catch (e: Exception) {}
        // the phone's own frame must not be accepted by the phone (reflection)
        try { phone.open(phone.frame("x".toByteArray()).copyOfRange(4, 4 + 17)); fail("reflection accepted") } catch (e: Exception) {}
        // PC session decrypts the phone's second frame only in order
        val f2 = pc.open(hex("000000159578d0bd15a7d0bc814ee4f79ee8f74a3b73f02482").let { it.copyOfRange(4, it.size) })
        assertEquals("hello", String(f2))
    }
}
