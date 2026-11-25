package com.ibrahim.phonelink

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.net.Uri
import android.os.Handler
import android.os.Looper
import android.provider.OpenableColumns
import android.util.Base64
import org.json.JSONObject
import java.io.DataInputStream
import java.io.File
import java.io.FileOutputStream
import java.net.InetSocketAddress
import java.net.Socket
import java.security.*
import java.security.interfaces.ECPublicKey
import java.security.spec.*
import java.math.BigInteger
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import javax.crypto.Cipher
import javax.crypto.KeyAgreement
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.SecretKeySpec

/** Single connection to the Windows app. Protocol is described in windows/crypto.go. */
object Link {
    const val PORT = 47800
    @Volatile var status = "Not connected"
    @Volatile var sasCode: String? = null
    @Volatile var onChange: (() -> Unit)? = null
    private var out: java.io.OutputStream? = null
    private var key: SecretKeySpec? = null
    private var sock: Socket? = null
    private var confirm: CountDownLatch? = null
    @Volatile private var approved = false
    private val rnd = SecureRandom()
    private val main = Handler(Looper.getMainLooper())
    @Volatile private var lastRecvClip: String? = null
    private val rfiles = HashMap<String, FileOutputStream>()

    private fun setStatus(s: String) { status = s; main.post { onChange?.invoke() } }

    fun approve(ok: Boolean) { approved = ok; confirm?.countDown() }

    private fun prefs(c: Context) = c.getSharedPreferences("link", Context.MODE_PRIVATE)
    private val params: ECParameterSpec by lazy {
        val kpg = KeyPairGenerator.getInstance("EC"); kpg.initialize(ECGenParameterSpec("secp256r1"))
        (kpg.generateKeyPair().public as ECPublicKey).params
    }

    private fun myKeys(c: Context): KeyPair {
        val p = prefs(c); val kf = KeyFactory.getInstance("EC")
        val priv = p.getString("priv", null); val pub = p.getString("pub", null)
        if (priv != null && pub != null) {
            return KeyPair(kf.generatePublic(X509EncodedKeySpec(Base64.decode(pub, 0))),
                kf.generatePrivate(PKCS8EncodedKeySpec(Base64.decode(priv, 0))))
        }
        val kpg = KeyPairGenerator.getInstance("EC"); kpg.initialize(ECGenParameterSpec("secp256r1"))
        val kp = kpg.generateKeyPair()
        p.edit().putString("priv", Base64.encodeToString(kp.private.encoded, 0))
            .putString("pub", Base64.encodeToString(kp.public.encoded, 0)).apply()
        return kp
    }

    private fun raw65(k: PublicKey): ByteArray {
        val w = (k as ECPublicKey).w
        fun fix(b: BigInteger): ByteArray { val a = b.toByteArray(); val r = ByteArray(32)
            val n = minOf(32, a.size); System.arraycopy(a, a.size - n, r, 32 - n, n); return r }
        return byteArrayOf(4) + fix(w.affineX) + fix(w.affineY)
    }

    private fun fromRaw65(b: ByteArray): PublicKey {
        val x = BigInteger(1, b.copyOfRange(1, 33)); val y = BigInteger(1, b.copyOfRange(33, 65))
        return KeyFactory.getInstance("EC").generatePublic(ECPublicKeySpec(ECPoint(x, y), params))
    }

    private fun sha(vararg parts: ByteArray): ByteArray {
        val m = MessageDigest.getInstance("SHA-256"); parts.forEach { m.update(it) }; return m.digest()
    }

    fun connect(c: Context, host: String) {
        Thread {
            try {
                disconnect()
                setStatus("Connecting...")
                val s = Socket(); s.connect(InetSocketAddress(host, PORT), 5000); s.tcpNoDelay = true
                sock = s
                val kp = myKeys(c); val myPub = raw65(kp.public)
                val nc = ByteArray(16); rnd.nextBytes(nc)
                val os = s.getOutputStream(); val ins = DataInputStream(s.getInputStream())
                os.write(myPub + nc); os.flush()
                val resp = ByteArray(65 + 16); ins.readFully(resp)
                val srvPub = resp.copyOfRange(0, 65); val ns = resp.copyOfRange(65, 81)
                val ka = KeyAgreement.getInstance("ECDH"); ka.init(kp.private); ka.doPhase(fromRaw65(srvPub), true)
                val shared = ka.generateSecret()
                key = SecretKeySpec(sha("phonelink-v2".toByteArray(), shared, nc, ns).copyOf(16), "AES")
                val trusted = prefs(c).getString("pc", null)
                val srvB64 = Base64.encodeToString(srvPub, 0)
                if (trusted != null && trusted != srvB64) { setStatus("REFUSED: this PC is not the one you paired with"); s.close(); return@Thread }
                if (trusted == null) {
                    val h = sha("phonelink-sas".toByteArray(), myPub, srvPub)
                    val n = ((h[0].toLong() and 255) shl 24 or ((h[1].toLong() and 255) shl 16) or ((h[2].toLong() and 255) shl 8) or (h[3].toLong() and 255)) % 1000000
                    sasCode = "%06d".format(n)
                    approved = false; confirm = CountDownLatch(1)
                    setStatus("Compare code with PC: $sasCode")
                    if (!confirm!!.await(90, TimeUnit.SECONDS) || !approved) { sasCode = null; setStatus("Pairing cancelled"); s.close(); return@Thread }
                    sasCode = null
                    prefs(c).edit().putString("pc", srvB64).apply()
                }
                out = os
                send(JSONObject().put("t", "hello"))
                setStatus("Connected")
                prefs(c).edit().putString("host", host).apply()
                watchClipboard(c)
                while (true) handle(c, JSONObject(String(readFrame(ins))))
            } catch (e: Exception) {
                setStatus("Disconnected: ${e.message}")
            } finally { out = null }
        }.start()
    }

    fun disconnect() { try { sock?.close() } catch (_: Exception) {}; out = null }
    val connected get() = out != null

    @Synchronized fun send(j: JSONObject) {
        val o = out ?: return; val k = key ?: return
        try {
            val nonce = ByteArray(12); rnd.nextBytes(nonce)
            val ci = Cipher.getInstance("AES/GCM/NoPadding"); ci.init(Cipher.ENCRYPT_MODE, k, GCMParameterSpec(128, nonce))
            val ct = ci.doFinal(j.toString().toByteArray())
            val len = 12 + ct.size
            o.write(byteArrayOf((len shr 24).toByte(), (len shr 16).toByte(), (len shr 8).toByte(), len.toByte()) + nonce + ct)
            o.flush()
        } catch (e: Exception) { disconnect() }
    }

    private fun readFrame(ins: DataInputStream): ByteArray {
        val len = ins.readInt(); if (len < 28 || len > (1 shl 20)) throw Exception("bad frame")
        val b = ByteArray(len); ins.readFully(b)
        val ci = Cipher.getInstance("AES/GCM/NoPadding")
        ci.init(Cipher.DECRYPT_MODE, key, GCMParameterSpec(128, b, 0, 12))
        return ci.doFinal(b, 12, len - 12)
    }

    private fun handle(c: Context, m: JSONObject) {
        when (m.optString("t")) {
            "clip" -> { val t = m.optString("text"); lastRecvClip = t
                main.post { (c.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager).setPrimaryClip(ClipData.newPlainText("pc", t)) } }
            "file" -> recvFile(c, m)
        }
    }

    private var clipListener: ClipboardManager.OnPrimaryClipChangedListener? = null
    private fun watchClipboard(c: Context) {
        main.post {
            val cm = c.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
            clipListener?.let { cm.removePrimaryClipChangedListener(it) }
            clipListener = ClipboardManager.OnPrimaryClipChangedListener {
                val t = cm.primaryClip?.getItemAt(0)?.coerceToText(c)?.toString()
                if (!t.isNullOrEmpty() && t != lastRecvClip) Thread { send(JSONObject().put("t", "clip").put("text", t)) }.start()
            }
            cm.addPrimaryClipChangedListener(clipListener)
        }
    }

    fun sendNotification(app: String, title: String, text: String) {
        if (connected) send(JSONObject().put("t", "notif").put("app", app).put("title", title).put("text", text))
    }

    fun receivedDir(c: Context): File = File(c.getExternalFilesDir(null), "Received").also { it.mkdirs() }

    private fun recvFile(c: Context, m: JSONObject) {
        val name = File(m.optString("name")).name; if (name.isEmpty()) return
        val f = rfiles.getOrPut(name) { FileOutputStream(File(receivedDir(c), name)) }
        f.write(Base64.decode(m.optString("data"), Base64.NO_WRAP))
        if (m.optBoolean("last")) { f.close(); rfiles.remove(name); setStatus("Received file: $name") }
    }

    fun sendFile(c: Context, uri: Uri) {
        Thread {
            try {
                var name = "file"
                c.contentResolver.query(uri, null, null, null, null)?.use {
                    if (it.moveToFirst()) { val i = it.getColumnIndex(OpenableColumns.DISPLAY_NAME); if (i >= 0) name = it.getString(i) }
                }
                c.contentResolver.openInputStream(uri)!!.use { ins ->
                    val buf = ByteArray(192 * 1024)
                    while (true) {
                        var n = 0
                        while (n < buf.size) { val r = ins.read(buf, n, buf.size - n); if (r < 0) break; n += r }
                        val last = n < buf.size
                        send(JSONObject().put("t", "file").put("name", name)
                            .put("data", Base64.encodeToString(buf, 0, n, Base64.NO_WRAP)).put("last", last))
                        if (last) break
                    }
                }
                setStatus("Sent file: $name")
            } catch (e: Exception) { setStatus("Send failed: ${e.message}") }
        }.start()
    }
}
