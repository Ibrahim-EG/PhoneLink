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

# ---- end to end: the real PC app (Go, Linux build with auto-accept) <-> the app on the emulator ----
export HOME="$PWD/pchome"; mkdir -p "$HOME"
./pc-e2e > shots/pc.log 2>&1 &
PCPID=$!
sleep 2
kill -0 $PCPID || fail "PC app did not start"
python3 .github/e2e.py type_host 10.0.2.2 || fail "could not type address"
python3 .github/e2e.py tap Connect || fail "no Connect button"
CODE=$(python3 .github/e2e.py code) || fail "no pairing code on phone"
adb exec-out screencap -p > shots/pairing.png
sleep 1
grep -q "Confirmation code: $CODE" shots/pc.log || { cat shots/pc.log; fail "phone code $CODE does not match the PC code"; }
echo "pairing code matched on both sides: $CODE"
python3 .github/e2e.py tap "Codes match" || fail "no Codes match button"
python3 .github/e2e.py wait "Connected" 40 || fail "phone never reported connected"
sleep 2
adb exec-out screencap -p > shots/connected.png
grep -q "Phone connected" shots/pc.log || { cat shots/pc.log; fail "PC did not log a connected phone"; }
# PC -> phone file: drop a file into the PC Send folder and wait for the phone to confirm receipt
mkdir -p "$HOME/Downloads/PhoneLink/Send"
echo "hello from the pc" > "$HOME/Downloads/PhoneLink/Send/e2e-hello.txt"
python3 .github/e2e.py wait "Received file" 60 || { cat shots/pc.log; fail "phone never received the file"; }
adb exec-out screencap -p > shots/file-received.png
for i in $(seq 1 20); do grep -q "Delivered (confirmed by phone)" shots/pc.log && break; sleep 1; done
grep -q "Delivered (confirmed by phone)" shots/pc.log || { cat shots/pc.log; fail "PC never got the delivery confirmation"; }
adb shell ls /sdcard/Android/data/com.ibrahim.phonelink/files/Received | tee shots/phone-files.txt
grep -q e2e-hello shots/phone-files.txt || fail "file not on the phone's disk"
echo "OK end to end: pairing, connect, PC->phone file with confirmation"
kill $PCPID
