package com.ibrahim.phonelink

import java.io.DataInputStream
import java.io.OutputStream
import java.math.BigInteger
import java.security.*
import java.security.interfaces.ECPublicKey
import java.security.spec.*
import javax.crypto.Cipher
import javax.crypto.KeyAgreement
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.SecretKeySpec

/** Pure JVM protocol code (no Android classes) so it can be unit-tested. Mirrors windows/crypto.go. */
object Proto {
    private val params: ECParameterSpec by lazy {
        val kpg = KeyPairGenerator.getInstance("EC"); kpg.initialize(ECGenParameterSpec("secp256r1"))
        (kpg.generateKeyPair().public as ECPublicKey).params
    }

    fun sha(vararg parts: ByteArray): ByteArray {
        val m = MessageDigest.getInstance("SHA-256"); parts.forEach { m.update(it) }; return m.digest()
    }

    fun raw65(k: PublicKey): ByteArray {
        val w = (k as ECPublicKey).w
        fun fix(b: BigInteger): ByteArray {
            val a = b.toByteArray(); val r = ByteArray(32)
            val n = minOf(32, a.size); System.arraycopy(a, a.size - n, r, 32 - n, n); return r
        }
        return byteArrayOf(4) + fix(w.affineX) + fix(w.affineY)
    }

    fun fromRaw65(b: ByteArray): PublicKey {
        require(b.size == 65 && b[0] == 4.toByte())
        val x = BigInteger(1, b.copyOfRange(1, 33)); val y = BigInteger(1, b.copyOfRange(33, 65))
        return KeyFactory.getInstance("EC").generatePublic(ECPublicKeySpec(ECPoint(x, y), params))
    }

    fun privFromScalar(d: ByteArray): PrivateKey =
        KeyFactory.getInstance("EC").generatePrivate(ECPrivateKeySpec(BigInteger(1, d), params))

    fun ecdh(priv: PrivateKey, peer: PublicKey): ByteArray {
        val ka = KeyAgreement.getInstance("ECDH"); ka.init(priv); ka.doPhase(peer, true); return ka.generateSecret()
    }

    fun commit(pub: ByteArray, nonce: ByteArray) = sha("phonelink-commit".toByteArray(), pub, nonce)

    fun sas(clientPub: ByteArray, serverPub: ByteArray, nc: ByteArray, ns: ByteArray): String {
        val h = sha("phonelink-sas".toByteArray(), clientPub, serverPub, nc, ns)
        val n = ((h[0].toLong() and 255) shl 24 or ((h[1].toLong() and 255) shl 16) or
            ((h[2].toLong() and 255) shl 8) or (h[3].toLong() and 255)) % 1000000
        return "%06d".format(n)
    }

    fun keyBytes(shared: ByteArray, nc: ByteArray, ns: ByteArray) =
        sha("phonelink-v2".toByteArray(), shared, nc, ns).copyOf(16)

    /** dir: 0 = phone (client) sends, 1 = PC (server) sends. Counters make replay/drop/reorder fail. */
    class Session(keyBytes: ByteArray, private val dir: Int) {
        private val key = SecretKeySpec(keyBytes, "AES")
        private var sn = 0L
        private var rn = 0L

        private fun nonce(d: Int, n: Long): ByteArray {
            val b = ByteArray(12); b[0] = d.toByte()
            for (i in 0 until 8) b[4 + i] = (n shr (56 - 8 * i)).toByte()
            return b
        }

        @Synchronized fun frame(msg: ByteArray): ByteArray {
            val c = Cipher.getInstance("AES/GCM/NoPadding")
            c.init(Cipher.ENCRYPT_MODE, key, GCMParameterSpec(128, nonce(dir, sn)))
            val ct = c.doFinal(msg); sn++
            val len = ct.size
            return byteArrayOf((len shr 24).toByte(), (len shr 16).toByte(), (len shr 8).toByte(), len.toByte()) + ct
        }

        fun write(o: OutputStream, msg: ByteArray) { synchronized(o) { val f = frame(msg); o.write(f); o.flush() } }  // counter order = wire order

        fun read(ins: DataInputStream): ByteArray {
            val len = ins.readInt(); if (len < 16 || len > (1 shl 20)) throw Exception("bad frame")
            val b = ByteArray(len); ins.readFully(b)
            return open(b)
        }

        /** Counter advances only when authentication succeeds. */
        fun open(ct: ByteArray): ByteArray {
            val c = Cipher.getInstance("AES/GCM/NoPadding")
            c.init(Cipher.DECRYPT_MODE, key, GCMParameterSpec(128, nonce(1 - dir, rn)))
            val p = c.doFinal(ct); rn++
            return p
        }
    }
}
