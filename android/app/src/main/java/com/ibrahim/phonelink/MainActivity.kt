package com.ibrahim.phonelink

import android.app.Activity
import android.content.Intent
import android.os.Bundle
import android.provider.Settings
import android.view.ViewGroup
import android.widget.*

class MainActivity : Activity() {
    private lateinit var status: TextView
    private lateinit var confirmRow: LinearLayout

    override fun onCreate(s: Bundle?) {
        super.onCreate(s)
        val l = LinearLayout(this); l.orientation = LinearLayout.VERTICAL; l.setPadding(32, 32, 32, 32)
        val host = EditText(this); host.hint = "PC address (e.g. 192.168.1.20)"
        host.setText(getSharedPreferences("link", MODE_PRIVATE).getString("host", ""))
        status = TextView(this); status.textSize = 18f
        fun btn(t: String, a: () -> Unit) = Button(this).also { it.text = t; it.setOnClickListener { a() }; l.addView(it) }
        l.addView(host)
        btn("Connect") { startService(Intent(this, LinkService::class.java)); Link.connect(applicationContext, host.text.toString().trim().substringBefore(":")) }
        btn("Disconnect") { Link.stop() }
        btn("Send a file to PC") { startActivityForResult(Intent(Intent.ACTION_OPEN_DOCUMENT).setType("*/*").addCategory(Intent.CATEGORY_OPENABLE), 1) }
        btn("Allow notification sharing") { startActivity(Intent("android.settings.ACTION_NOTIFICATION_LISTENER_SETTINGS")) }
        confirmRow = LinearLayout(this)
        confirmRow.addView(Button(this).also { it.text = "Codes match"; it.setOnClickListener { Link.approve(true) } })
        confirmRow.addView(Button(this).also { it.text = "Cancel"; it.setOnClickListener { Link.approve(false) } })
        l.addView(status); l.addView(confirmRow)
        val info = TextView(this)
        info.text = "Received files: Android/data/com.ibrahim.phonelink/files/Received"
        l.addView(info)
        setContentView(l, ViewGroup.LayoutParams(-1, -1))
        Link.onChange = { refresh() }; refresh()
    }

    private fun refresh() {
        status.text = Link.status
        confirmRow.visibility = if (Link.sasCode != null) android.view.View.VISIBLE else android.view.View.GONE
    }

    override fun onActivityResult(r: Int, c: Int, d: Intent?) {
        if (r == 1 && c == RESULT_OK && d?.data != null) Link.sendFile(applicationContext, d.data!!)
    }
}
