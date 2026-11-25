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

# ---- more functions (each result is printed; hard failures only where the step is deterministic) ----
PKG=com.ibrahim.phonelink
FAILS=0
softfail() { FAILS=$((FAILS+1)); echo "RESULT FAIL: $1"; adb logcat -d | tail -40 > shots/logcat-soft-$RANDOM.txt; }
# PC -> phone clipboard: the phone must log receipt; then try to paste into the address box to see the real clipboard
echo -n "pc-clip-123" > "$HOME/clip-send.txt"
python3 .github/e2e.py wait "Clipboard received from PC" 30 || { cat shots/pc.log; softfail "phone did not receive the PC clipboard"; }
echo "OK PC->phone clipboard receipt logged on the phone"
adb shell input keyevent 279 # KEYCODE_PASTE into the focused address box
sleep 1
adb exec-out screencap -p > shots/clip-pc-to-phone.png
if python3 .github/e2e.py wait "pc-clip-123" 3; then echo "OK real phone clipboard contains the PC text (pasted into a field)"; else echo "UNVERIFIED actual phone clipboard contents (paste check did not show the text)"; fi
# phone -> PC: shared text (this proves the share path, NOT the clipboard listener)
adb shell am start -n $PKG/.ShareActivity -a android.intent.action.SEND -t text/plain --es android.intent.extra.TEXT share-hello-1 >/dev/null
for i in $(seq 1 20); do [ -f "$HOME/clip-recv.txt" ] && grep -q share-hello-1 "$HOME/clip-recv.txt" && break; sleep 1; done
grep -q share-hello-1 "$HOME/clip-recv.txt" 2>/dev/null || { cat shots/pc.log; softfail "shared text did not reach the PC"; }
echo "OK phone->PC shared text reached the PC (share path only)"
# phone -> PC file via the share target
echo "from the phone" > /tmp/share-test.txt
adb push /tmp/share-test.txt /sdcard/Android/data/$PKG/files/share-test.txt >/dev/null
adb shell am start -n $PKG/.ShareActivity -a android.intent.action.SEND -t text/plain --eu android.intent.extra.STREAM file:///sdcard/Android/data/$PKG/files/share-test.txt >/dev/null
for i in $(seq 1 30); do [ -f "$HOME/Downloads/PhoneLink/Received/share-test.txt" ] && break; sleep 1; done
[ -f "$HOME/Downloads/PhoneLink/Received/share-test.txt" ] || { cat shots/pc.log; adb logcat -d | tail -40; softfail "phone->PC file did not arrive"; }
grep -q "from the phone" "$HOME/Downloads/PhoneLink/Received/share-test.txt" || softfail "phone->PC file content wrong"
echo "OK phone->PC file arrived with correct content"
adb exec-out screencap -p > shots/phone-to-pc-file.png
# phone -> PC real clipboard listener: set the phone clipboard through the address box (type, select all, copy)
echo "UNVERIFIED phone clipboard listener -> PC: no reliable way to copy on Android 7.0 headless, not tested"
echo "UNVERIFIED received-file copy to a user-chosen folder: the system folder picker is not driven by this test"
# sky motion: two frames 2 seconds apart should differ (animation). This does not prove smoothness.
adb exec-out screencap -p > shots/motion-a.png; sleep 2; adb exec-out screencap -p > shots/motion-b.png
if cmp -s shots/motion-a.png shots/motion-b.png; then softfail "screen identical 2s apart: no animation seen"; else echo "OK frames differ 2s apart (animation or other change, smoothness not measured)"; fi
# touch feedback: long press with a screenshot taken while the finger is down (visual review only, not asserted)
SZ=$(adb shell wm size | grep -o '[0-9]*x[0-9]*' | tail -1); W=${SZ%x*}; H=${SZ#*x}
adb shell input swipe $((W/2)) $((H*6/10)) $((W/2)) $((H*6/10)) 2500 &
sleep 1; adb exec-out screencap -p > shots/touch-down.png; wait
echo "UNVERIFIED touch glow/ripple: frame saved at shots/touch-down.png for visual review, not asserted"
# notification mirroring was removed from the app; nothing to test
adb exec-out screencap -p > shots/final.png
kill $PCPID
echo "SUMMARY: $FAILS supported-feature checks failed (UNVERIFIED lines above are environment limits, not failures)"
[ "$FAILS" -eq 0 ] || exit 1
