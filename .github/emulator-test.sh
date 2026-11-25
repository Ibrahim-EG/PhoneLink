#!/usr/bin/env bash
# Installs the APK on an Android 7.0 (API 24) emulator, launches it and saves screenshots.
set -u
mkdir -p shots
fail() { echo "FAIL: $1"; adb logcat -d | tail -80 > shots/logcat.txt; adb exec-out screencap -p > shots/fail.png; exit 1; }
adb shell getprop ro.build.version.release | tee shots/android-version.txt
start=$(date +%s)
timeout 240 adb install -r apk/PhoneLink.apk 2>&1 | tee shots/install.txt
grep -q Success shots/install.txt || fail "install did not report Success"
echo "install took $(( $(date +%s) - start )) s"
adb shell pm list packages | grep -q com.ibrahim.phonelink || fail "package not installed"
adb shell monkey -p com.ibrahim.phonelink -c android.intent.category.LAUNCHER 1 >/dev/null 2>&1
sleep 6
adb exec-out screencap -p > shots/launch.png
adb shell pidof com.ibrahim.phonelink || fail "app process not running after launch"
adb logcat -d | grep -i "FATAL EXCEPTION" && fail "crash after launch"
echo "OK installed and launched"
