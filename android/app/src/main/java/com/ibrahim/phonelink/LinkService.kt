package com.ibrahim.phonelink

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.Service
import android.content.Intent
import android.os.Build
import android.os.IBinder

/** Keeps the process alive in the background so clipboard sync keeps working. */
class LinkService : Service() {
    override fun onBind(i: Intent?): IBinder? = null
    override fun onStartCommand(i: Intent?, f: Int, id: Int): Int {
        val nm = getSystemService(NOTIFICATION_SERVICE) as NotificationManager
        val b: Notification.Builder = if (Build.VERSION.SDK_INT >= 26) {
            nm.createNotificationChannel(NotificationChannel("link", "PhoneLink", NotificationManager.IMPORTANCE_MIN))
            Notification.Builder(this, "link")
        } else Notification.Builder(this).setPriority(Notification.PRIORITY_MIN)
        startForeground(1, b.setContentTitle("PhoneLink is running").setSmallIcon(android.R.drawable.stat_notify_sync).build())
        return START_STICKY
    }
}
