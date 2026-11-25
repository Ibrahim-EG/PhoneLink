package com.ibrahim.phonelink

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.net.Uri
import android.os.Handler
import android.os.Looper
import android.provider.DocumentsContract
import android.provider.OpenableColumns
import android.util.Base64
import org.json.JSONObject
import java.io.DataInputStream
import java.io.File
import java.io.FileOutputStream
import java.net.InetSocketAddress
import java.net.Socket
import java.security.*
import java.security.spec.*
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit

/** Single connection to the Windows app. Protocol is described in windows/crypto.go; crypto lives in Proto.kt. */
object Link {
    const val PORT = 47800
    @Volatile var status = "Not connected"
    @Volatile var sasCode: String? = null
    @Volatile var onChange: (() -> Unit)? = null
    private var out: java.io.OutputStream? = null
    private var session: Proto.Session? = null
    private var sock: Socket? = null
    private var confirm: CountDownLatch? = null
    @Volatile private var approved = false
    private val rnd = SecureRandom()
    private val main = Handler(Looper.getMainLooper())
    @Volatile private var lastRecvClip: String? = null
    private val acks = AckTracker()
    private val doneIds = DoneIds()
    private class Partial(val os: FileOutputStream, val file: File)
    private val rfiles = HashMap<String, Partial>()

    val events = java.util.concurrent.CopyOnWriteArrayList<String>()
    fun note(s: String) {
        events.add(0, java.text.SimpleDateFormat("HH:mm:ss", java.util.Locale.US).format(java.util.Date()) + "  " + s)
        while (events.size > 30) events.removeAt(events.size - 1)
        onChange?.invoke()
    }
    /** Sends text (shared from another app) to the PC clipboard. */
    fun sendText(t: String): Boolean {
        if (!connected) return false
        Thread { send(JSONObject().put("t", "clip").put("text", t)) }.start()
        note("Shared text sent to PC (" + t.length + " characters)")
        return true
    }
    private fun setStat(s: String) {
        status = s
        events.add(0, java.text.SimpleDateFormat("HH:mm:ss", java.util.Locale.US).format(java.util.Date()) + "  " + s)
        while (events.size > 30) events.removeAt(events.size - 1)
        main.post { onChange?.invoke() }
    }

    fun approve(ok: Boolean) { approved = ok; confirm?.countDown() }

    private fun prefs(c: Context) = c.getSharedPreferences("link", Context.MODE_PRIVATE)

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

    @Volatile private var wantConn = false
    @Volatile private var running = false

    /** Keeps trying every 5 s until the user taps Disconnect or pairing is refused. */
    fun connect(c: Context, host: String) {
        wantConn = true
        if (running) return
        running = true
        Thread {
            while (wantConn) {
                val retry = attempt(c, host)
                if (!retry) break
                try { Thread.sleep(5000) } catch (_: InterruptedException) {}
            }
            running = false
        }.start()
    }

    fun forgetPc(c: Context) { stop(); prefs(c).edit().remove("pc").apply(); setStat("Forgot PC. Connect to pair again.") }

    fun stop() { wantConn = false; disconnect(); setStat("Disconnected") }

    /** One connection attempt. Returns true when a retry makes sense. */
    private fun attempt(c: Context, host: String): Boolean {
        try {
            disconnect()
            setStat("Connecting...")
            val s = Socket(); s.connect(InetSocketAddress(host, PORT), 5000); s.tcpNoDelay = true
            sock = s
            val kp = myKeys(c); val myPub = Proto.raw65(kp.public)
            val nc = ByteArray(16); rnd.nextBytes(nc)
            val os = s.getOutputStream(); val ins = DataInputStream(s.getInputStream())
            os.write(Proto.commit(myPub, nc)); os.flush()          // commit first ...
            val resp = ByteArray(65 + 16); ins.readFully(resp)      // ... see the PC's key ...
            os.write(myPub + nc); os.flush()                        // ... then reveal ours
            val srvPub = resp.copyOfRange(0, 65); val ns = resp.copyOfRange(65, 81)
            val shared = Proto.ecdh(kp.private, Proto.fromRaw65(srvPub))
            val sess = Proto.Session(Proto.keyBytes(shared, nc, ns), 0)
            val trusted = prefs(c).getString("pc", null)
            val srvB64 = Base64.encodeToString(srvPub, 0)
            if (trusted != null && trusted != srvB64) { setStat("REFUSED: this PC is not the one you paired with. Use Forget paired PC if you reinstalled it."); s.close(); return false }
            if (trusted == null) {
                sasCode = Proto.sas(myPub, srvPub, nc, ns)
                approved = false; confirm = CountDownLatch(1)
                setStat("Compare code with PC: $sasCode")
                if (!confirm!!.await(90, TimeUnit.SECONDS) || !approved) { sasCode = null; setStat("Pairing cancelled"); s.close(); return false }
                sasCode = null
                prefs(c).edit().putString("pc", srvB64).apply()
            }
            session = sess; out = os
            send(JSONObject().put("t", "hello"))
            setStat("Connected")
            prefs(c).edit().putString("host", host).apply()
            watchClipboard(c)
            while (true) handle(c, JSONObject(String(sess.read(ins))))
        } catch (e: Exception) {
            setStat("Disconnected, retrying... (${e.message})")
        } finally { out = null; session = null; abortReceives() }
        return true
    }

    fun disconnect() { try { sock?.close() } catch (_: Exception) {}; out = null }
    val connected get() = out != null

    /** Returns false if the message could not be written. */
    fun send(j: JSONObject): Boolean {
        val o = out ?: return false; val s = session ?: return false
        return try { s.write(o, j.toString().toByteArray()); true } catch (e: Exception) { disconnect(); false }
    }

    private fun handle(c: Context, m: JSONObject) {
        when (m.optString("t")) {
            "clip" -> { val t = m.optString("text"); lastRecvClip = t; note("Clipboard received from PC (" + t.length + " characters)")
                main.post { (c.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager).setPrimaryClip(ClipData.newPlainText("pc", t)) } }
            "file" -> recvFile(c, m)
            "ack" -> acks.onAck(m.optString("id"), m.optBoolean("ok"))
        }
    }

    private var clipListener: ClipboardManager.OnPrimaryClipChangedListener? = null
    private fun watchClipboard(c: Context) {
        main.post {
            val cm = c.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
            clipListener?.let { cm.removePrimaryClipChangedListener(it) }
            clipListener = ClipboardManager.OnPrimaryClipChangedListener {
                val t = cm.primaryClip?.getItemAt(0)?.coerceToText(c)?.toString()
                if (!t.isNullOrEmpty() && t != lastRecvClip) { note("Clipboard sent to PC (" + t.length + " characters)"); Thread { send(JSONObject().put("t", "clip").put("text", t)) }.start() }
            }
            cm.addPrimaryClipChangedListener(clipListener)
        }
    }

    /** Folder the user picked with the system folder picker (no storage permission). Null until chosen. */
    fun exportTree(c: Context): Uri? = prefs(c).getString("exportTree", null)?.let { Uri.parse(it) }
    fun setExportTree(c: Context, u: Uri) { prefs(c).edit().putString("exportTree", u.toString()).apply() }

    /** Copies a received file into the chosen folder. The app-storage copy is kept either way. */
    private fun exportCopy(c: Context, f: File): Boolean {
        val tree = exportTree(c) ?: return false
        return try {
            val parent = DocumentsContract.buildDocumentUriUsingTree(tree, DocumentsContract.getTreeDocumentId(tree))
            val doc = DocumentsContract.createDocument(c.contentResolver, parent, "application/octet-stream", f.name)
            if (doc == null) false else { c.contentResolver.openOutputStream(doc)!!.use { o -> f.inputStream().use { it.copyTo(o) } }; true }
        } catch (e: Exception) { false }
    }

    fun receivedDir(c: Context): File = File(c.getExternalFilesDir(null), "Received").also { it.mkdirs() }

    private fun uniqueFile(dir: File, name: String): File {
        var f = File(dir, name); if (!f.exists()) return f
        val dot = name.lastIndexOf('.'); val base = if (dot > 0) name.substring(0, dot) else name; val ext = if (dot > 0) name.substring(dot) else ""
        var i = 1; while (true) { f = File(dir, "$base ($i)$ext"); if (!f.exists()) return f; i++ }
    }

    /** Data goes to a hidden .part file; it gets its real name only after the last chunk. */
    private fun recvFile(c: Context, m: JSONObject) {
        val name = File(m.optString("name")).name; if (name.isEmpty() || name.endsWith(".part")) return
        val id = m.optString("id")
        synchronized(rfiles) {
            if (id.isNotEmpty() && doneIds.has(id)) {   // a retry of a finished transfer: confirm again, no duplicate
                if (m.optBoolean("last")) Thread { send(JSONObject().put("t", "ack").put("id", id).put("name", name).put("ok", true)) }.start()
                return
            }
            val p = rfiles.getOrPut(name) { val f = File.createTempFile(".incoming-", ".part", receivedDir(c)); Partial(FileOutputStream(f), f) }
            try { p.os.write(Base64.decode(m.optString("data"), Base64.NO_WRAP)) } catch (e: Exception) {
                p.os.close(); p.file.delete(); rfiles.remove(name); return
            }
            if (m.optBoolean("last")) {
                p.os.close(); rfiles.remove(name)
                val dest = uniqueFile(receivedDir(c), name)
                val saved = p.file.renameTo(dest)
                if (saved) { if (id.isNotEmpty()) doneIds.add(id)
                    val hasTree = exportTree(c) != null
                    val copied = hasTree && exportCopy(c, dest)
                    setStat("Received file: ${dest.name}" + (if (copied) " (also saved to your chosen folder)" else if (hasTree) " (could NOT copy to your chosen folder; kept in app storage)" else "")) } else p.file.delete()
                Thread { send(JSONObject().put("t", "ack").put("id", id).put("name", name).put("ok", saved)) }.start()
            }
        }
    }

    private fun abortReceives() {
        synchronized(rfiles) { rfiles.values.forEach { try { it.os.close() } catch (_: Exception) {}; it.file.delete() }; rfiles.clear() }
    }

    fun sendFile(c: Context, uri: Uri) {
        if (!connected) { setStat("Not connected. Connect first, then send."); return }
        Thread {
            try {
                var name = uri.lastPathSegment?.substringAfterLast('/')?.takeIf { it.isNotBlank() } ?: "file"
                val id = c.contentResolver.openInputStream(uri)!!.use { TransferId.ofStream(it) }
                val wait = acks.register(id)
                c.contentResolver.query(uri, null, null, null, null)?.use {
                    if (it.moveToFirst()) { val i = it.getColumnIndex(OpenableColumns.DISPLAY_NAME); if (i >= 0) it.getString(i)?.let { n -> if (n.isNotBlank()) name = n } }
                }
                c.contentResolver.openInputStream(uri)!!.use { ins ->
                    val buf = ByteArray(192 * 1024)
                    while (true) {
                        var n = 0
                        while (n < buf.size) { val r = ins.read(buf, n, buf.size - n); if (r < 0) break; n += r }
                        val last = n < buf.size
                        val ok = send(JSONObject().put("t", "file").put("name", name).put("id", id)
                            .put("data", Base64.encodeToString(buf, 0, n, Base64.NO_WRAP)).put("last", last))
                        if (!ok) { setStat("Send interrupted (connection lost): $name. Try again after it reconnects."); return@Thread }
                        if (last) break
                    }
                }
                // written to the connection is not delivered: wait for the PC's confirmation
                val confirmed = acks.await(id, wait, 30000)
                setStat(if (confirmed) "Delivered (confirmed by PC): $name" else "Sent but NOT confirmed by the PC: $name. Check the PC, send again if missing.")
            } catch (e: Exception) { setStat("Send failed: ${e.message}") }
        }.start()
    }
}
