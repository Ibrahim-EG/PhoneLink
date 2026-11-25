package com.ibrahim.phonelink

import android.content.Context
import android.graphics.*
import android.view.MotionEvent
import android.view.View
import kotlin.math.*

/** Slow aurora sky (deep blue to violet, teal/violet curtains, stars) with a soft glow that follows touch and a ripple on tap. */
class AuroraView(c: Context) : View(c) {
    private val p = Paint(Paint.ANTI_ALIAS_FLAG)
    private val start = System.nanoTime()
    private var tx = -1f; private var ty = -1f; private var px = 0.5f; private var py = 0.5f; private var act = 0f; private var actT = 0f
    private var rx = 0f; private var ry = 0f; private var rAge = 9f
    private val stars = Array(70) { floatArrayOf(Math.random().toFloat(), Math.random().toFloat() * 0.7f, Math.random().toFloat() * 6.28f) }

    fun touch(e: MotionEvent) {
        when (e.actionMasked) {
            MotionEvent.ACTION_DOWN -> { rx = e.x; ry = e.y; rAge = 0f; tx = e.x; ty = e.y; actT = 1f }
            MotionEvent.ACTION_MOVE -> { tx = e.x; ty = e.y; actT = 1f }
            MotionEvent.ACTION_UP, MotionEvent.ACTION_CANCEL -> actT = 0f
        }
    }

    private fun glow(cv: Canvas, cx: Float, cy: Float, r: Float, color: Int, a: Int) {
        p.shader = RadialGradient(cx, cy, r, intArrayOf(Color.argb(a, Color.red(color), Color.green(color), Color.blue(color)), Color.argb(0, Color.red(color), Color.green(color), Color.blue(color))), null, Shader.TileMode.CLAMP)
        cv.drawCircle(cx, cy, r, p); p.shader = null
    }

    override fun onDraw(cv: Canvas) {
        val w = width.toFloat(); val h = height.toFloat()
        val t = (System.nanoTime() - start) / 1e9f
        p.shader = LinearGradient(0f, 0f, 0f, h, intArrayOf(Color.rgb(8, 12, 44), Color.rgb(20, 14, 62), Color.rgb(34, 14, 74)), null, Shader.TileMode.CLAMP)
        cv.drawRect(0f, 0f, w, h, p); p.shader = null
        if (tx >= 0) { px += (tx / w - px) * 0.08f; py += (ty / h - py) * 0.08f }
        act += (actT - act) * 0.06f
        val teal = Color.rgb(13, 140, 191); val violet = Color.rgb(115, 46, 191); val green = Color.rgb(26, 255, 107)
        for (i in 0 until 3) {
            val cx = w * (0.5f + 0.42f * sin(t * 0.07f + i * 2.1f))
            val cy = h * (0.18f + 0.22f * i + 0.05f * sin(t * 0.11f + i))
            glow(cv, cx, cy, w * 0.9f, if (i == 1) violet else teal, 70)
        }
        glow(cv, w * (0.3f + 0.4f * sin(t * 0.05f)), h * 0.12f, w * 0.5f, green, 22)
        if (act > 0.02f) glow(cv, px * w, py * h, w * 0.6f, Color.rgb(110, 200, 255), (46 * act).toInt())
        p.color = Color.WHITE
        for (s in stars) {
            p.alpha = (40 + 90 * (0.5f + 0.5f * sin(t * 2f + s[2]))).toInt()
            cv.drawCircle(s[0] * w, s[1] * h, 1.4f, p)
        }
        if (rAge < 1.6f) {
            rAge += 0.016f * 1.3f
            p.style = Paint.Style.STROKE; p.strokeWidth = 3f; p.color = Color.argb(((1f - rAge / 1.6f) * 110).toInt().coerceIn(0, 255), 170, 220, 255)
            cv.drawCircle(rx, ry, rAge * w * 0.5f, p); p.style = Paint.Style.FILL
        }
        postInvalidateOnAnimation()
    }
}
