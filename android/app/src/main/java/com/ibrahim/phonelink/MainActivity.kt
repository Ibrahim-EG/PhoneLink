package com.ibrahim.phonelink

import android.app.Activity
import android.content.Intent
import android.graphics.Color
import android.graphics.Typeface
import android.graphics.drawable.GradientDrawable
import android.os.Bundle
import android.text.InputType
import android.view.Gravity
import android.view.View
import android.widget.*

class MainActivity : Activity() {
    private lateinit var status: TextView
    private lateinit var dot: View
    private lateinit var aurora: AuroraView
    override fun dispatchTouchEvent(ev: android.view.MotionEvent): Boolean { if (::aurora.isInitialized) aurora.touch(ev); return super.dispatchTouchEvent(ev) }
    private lateinit var pairCard: LinearLayout
    private lateinit var codeView: TextView
    private lateinit var activity: LinearLayout
    private lateinit var disconnectBtn: Button

    private val blue = Color.parseColor("#6FB6FF")
    private val ink = Color.parseColor("#E4F0FF")
    private val muted = Color.parseColor("#A8B8DC")

    private fun dp(v: Int) = (v * resources.displayMetrics.density).toInt()
    private fun bg(color: Int, radius: Int, stroke: Int = 0) = GradientDrawable().also {
        it.setColor(color); it.cornerRadius = dp(radius).toFloat(); if (stroke != 0) it.setStroke(dp(1), stroke)
    }
    private fun card(parent: LinearLayout): LinearLayout {
        val c = LinearLayout(this); c.orientation = LinearLayout.VERTICAL
        c.background = bg(Color.parseColor("#C0120F2E"), 16, Color.parseColor("#4A4690")); c.setPadding(dp(16), dp(14), dp(16), dp(16))
        val lp = LinearLayout.LayoutParams(-1, -2); lp.topMargin = dp(12); parent.addView(c, lp); return c
    }
    private fun label(parent: LinearLayout, t: String) {
        val v = TextView(this); v.text = t; v.textSize = 13f; v.setTextColor(muted); v.typeface = Typeface.DEFAULT_BOLD
        parent.addView(v)
    }
    private fun button(t: String, primary: Boolean, a: () -> Unit): Button {
        val b = Button(this); b.text = t; b.isAllCaps = false; b.textSize = 15f; b.stateListAnimator = null
        b.setTextColor(if (primary) Color.parseColor("#0A0A2A") else blue)
        b.background = if (primary) bg(blue, 12) else bg(Color.parseColor("#33000000"), 12, blue)
        b.setOnClickListener { a() }; return b
    }
    private fun row(parent: LinearLayout, vararg bs: Button) {
        val r = LinearLayout(this); val lp0 = LinearLayout.LayoutParams(-1, -2); lp0.topMargin = dp(10); parent.addView(r, lp0)
        bs.forEachIndexed { i, b ->
            val lp = LinearLayout.LayoutParams(0, dp(46), 1f); if (i > 0) lp.leftMargin = dp(10); r.addView(b, lp)
        }
    }

    override fun onCreate(s: Bundle?) {
        super.onCreate(s)
        val scroll = ScrollView(this); scroll.setBackgroundColor(Color.TRANSPARENT)
        val root = LinearLayout(this); root.orientation = LinearLayout.VERTICAL; root.setPadding(dp(16), dp(20), dp(16), dp(24))
        scroll.addView(root)

        // header
        val head = LinearLayout(this); head.gravity = Gravity.CENTER_VERTICAL
        val icon = ImageView(this); icon.setImageResource(R.drawable.ic_launcher)
        head.addView(icon, LinearLayout.LayoutParams(dp(52), dp(52)))
        val titles = LinearLayout(this); titles.orientation = LinearLayout.VERTICAL; titles.setPadding(dp(14), 0, 0, 0)
        val title = TextView(this); title.text = "PhoneLink"; title.textSize = 24f; title.setTextColor(ink); title.typeface = Typeface.DEFAULT_BOLD
        val sub = TextView(this); sub.text = "Your phone and PC, same Wi-Fi"; sub.textSize = 13f; sub.setTextColor(muted)
        titles.addView(title); titles.addView(sub); head.addView(titles)
        root.addView(head)

        // status
        val st = card(root)
        val srow = LinearLayout(this); srow.gravity = Gravity.CENTER_VERTICAL
        dot = View(this); srow.addView(dot, LinearLayout.LayoutParams(dp(12), dp(12)))
        status = TextView(this); status.textSize = 16f; status.setTextColor(ink); status.setPadding(dp(12), 0, 0, 0)
        srow.addView(status); st.addView(srow)

        // connect
        val cc = card(root); label(cc, "PC ADDRESS")
        val host = EditText(this); host.hint = "e.g. 192.168.1.20"; host.inputType = InputType.TYPE_CLASS_TEXT or InputType.TYPE_TEXT_VARIATION_URI
        host.setSingleLine(); host.background = bg(Color.parseColor("#990A0922"), 10); host.setTextColor(ink); host.setHintTextColor(muted); host.setPadding(dp(12), dp(10), dp(12), dp(10))
        host.setText(getSharedPreferences("link", MODE_PRIVATE).getString("host", ""))
        val hlp = LinearLayout.LayoutParams(-1, -2); hlp.topMargin = dp(6); cc.addView(host, hlp)
        val conn = button("Connect", true) {
            startService(Intent(this, LinkService::class.java)); Link.connect(applicationContext, host.text.toString().trim().substringBefore(":"))
        }
        disconnectBtn = button("Disconnect", false) { Link.stop() }
        row(cc, conn, disconnectBtn)

        // pairing
        pairCard = card(root)
        label(pairCard, "PAIRING: CHECK THE CODE")
        codeView = TextView(this); codeView.textSize = 38f; codeView.typeface = Typeface.MONOSPACE; codeView.setTextColor(ink)
        codeView.gravity = Gravity.CENTER; codeView.letterSpacing = 0.15f; codeView.setPadding(0, dp(10), 0, dp(4))
        pairCard.addView(codeView)
        val hint = TextView(this); hint.text = "Only tap Codes match if the PC shows the same six digits."; hint.textSize = 13f; hint.setTextColor(muted); hint.gravity = Gravity.CENTER
        pairCard.addView(hint)
        row(pairCard, button("Codes match", true) { Link.approve(true) }, button("Cancel", false) { Link.approve(false) })

        // actions
        val ac = card(root); label(ac, "ACTIONS")
        row(ac, button("Send a file to PC", true) {
            startActivityForResult(Intent(Intent.ACTION_OPEN_DOCUMENT).setType("*/*").addCategory(Intent.CATEGORY_OPENABLE), 1)
        })
        row(ac, button("Allow notification sharing", false) { startActivity(Intent("android.settings.ACTION_NOTIFICATION_LISTENER_SETTINGS")) })
        val info = TextView(this); info.textSize = 12f; info.setTextColor(muted); info.setPadding(0, dp(10), 0, 0)
        info.text = "Files from the PC are saved in Android/data/com.ibrahim.phonelink/files/Received"
        ac.addView(info)

        // activity
        val lc = card(root); label(lc, "RECENT ACTIVITY")
        activity = LinearLayout(this); activity.orientation = LinearLayout.VERTICAL; lc.addView(activity)

        // footer
        val forget = button("Forget paired PC", false) { Link.forgetPc(applicationContext) }
        val flp = LinearLayout.LayoutParams(-1, dp(44)); flp.topMargin = dp(16); root.addView(forget, flp)

        aurora = AuroraView(this)
        val frame = FrameLayout(this); frame.addView(aurora, FrameLayout.LayoutParams(-1, -1)); frame.addView(scroll, FrameLayout.LayoutParams(-1, -1))
        setContentView(frame)
        Link.onChange = { refresh() }; refresh()
    }

    private fun refresh() {
        status.text = Link.status
        val ok = Link.connected
        dot.background = bg(if (ok) Color.parseColor("#22A559") else Color.parseColor("#7A86A8"), 6)
        disconnectBtn.isEnabled = ok
        disconnectBtn.alpha = if (ok) 1f else 0.45f
        val code = Link.sasCode
        pairCard.visibility = if (code != null) View.VISIBLE else View.GONE
        if (code != null) codeView.text = code
        activity.removeAllViews()
        val items = Link.events.toList().take(8)
        if (items.isEmpty()) activity.addView(TextView(this).also { it.text = "Nothing yet"; it.setTextColor(muted); it.textSize = 13f })
        for (e in items) activity.addView(TextView(this).also { it.text = e; it.setTextColor(ink); it.textSize = 13f; it.setPadding(0, dp(4), 0, dp(4)) })
    }

    override fun onActivityResult(r: Int, c: Int, d: Intent?) {
        if (r == 1 && c == RESULT_OK && d?.data != null) Link.sendFile(applicationContext, d.data!!)
    }
}
