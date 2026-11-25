package com.ibrahim.phonelink

import android.app.Activity
import android.content.Intent
import android.net.Uri
import android.os.Bundle
import android.widget.Toast

/** "Share > PhoneLink": sends shared files or text to the connected PC, then closes. */
class ShareActivity : Activity() {
    override fun onCreate(s: Bundle?) {
        super.onCreate(s)
        val i = intent
        var sent = 0
        if (!Link.connected) {
            Toast.makeText(this, "PhoneLink is not connected to the PC", Toast.LENGTH_LONG).show()
            finish(); return
        }
        val uris = ArrayList<Uri>()
        if (i.action == Intent.ACTION_SEND_MULTIPLE) {
            i.getParcelableArrayListExtra<Uri>(Intent.EXTRA_STREAM)?.let { uris.addAll(it) }
        } else {
            val u = i.getParcelableExtra<Uri>(Intent.EXTRA_STREAM)
            if (u != null) uris.add(u)
        }
        for (u in uris) { Link.sendFile(applicationContext, u); sent++ }
        if (uris.isEmpty()) {
            val t = i.getStringExtra(Intent.EXTRA_TEXT)
            if (!t.isNullOrEmpty() && Link.sendText(t)) sent++
        }
        Toast.makeText(this, if (sent > 0) "Sending to PC" else "Nothing to send", Toast.LENGTH_SHORT).show()
        finish()
    }
}
