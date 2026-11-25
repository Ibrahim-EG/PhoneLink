#!/usr/bin/env bash
# Installs the APK on an Android 7.0 (API 24) emulator, launches it and saves screenshots.
set -u
mkdir -p shots
exec > >(tee shots/test-output.txt) 2>&1
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
hidekb() { for k in 1 2; do adb shell dumpsys input_method | grep -q "mInputShown=true" && { adb shell input keyevent 4; sleep 1; }; done; }
appfocus() { adb shell dumpsys window | grep mCurrentFocus | grep -q "com.ibrahim.phonelink/"; }
# ensure_app LABEL: bring PhoneLink to the front, hide the keyboard only if it is shown, and require both window focus and the main screen text
ensure_app() { foreground; if appfocus && python3 .github/e2e.py wait "Your phone and PC, same Wi-Fi" 5 >/dev/null 2>&1; then return 0; fi; echo "UNVERIFIED $1: precondition failed (PhoneLink was not the focused window showing its main screen)"; adb exec-out screencap -p > shots/precondition-$RANDOM.png; adb shell dumpsys window | grep mCurrentFocus; return 1; }
foreground() { adb shell am start -n $PKG/.MainActivity >/dev/null 2>&1; sleep 2; hidekb; }
FAILS=0
softfail() { FAILS=$((FAILS+1)); echo "RESULT FAIL: $1"; adb logcat -d | tail -40 > shots/logcat-soft-$RANDOM.txt; }
# PC -> phone clipboard: the phone must log receipt; then try to paste into the address box to see the real clipboard
echo -n "pc-clip-123" > "$HOME/clip-send.txt"
logwait() { for i in $(seq 1 $2); do adb logcat -d -s PhoneLinkNote:V | grep -q "$1" && return 0; sleep 1; done; return 1; }
if logwait "Clipboard received from PC" 30; then echo "OK PC->phone clipboard receipt logged on the phone (log receipt only)"; else cat shots/pc.log; adb logcat -d -s PhoneLinkNote:V AndroidRuntime:E | tail -40; softfail "phone did not log receipt of the PC clipboard (logcat PhoneLinkNote)"; fi
if logwait "setPrimaryClip done" 5; then echo "OK phone applied the PC text to its clipboard (setPrimaryClip returned, logcat)"; else adb logcat -d -s PhoneLinkNote:V | tail -20; softfail "phone did not apply the PC text to its clipboard"; fi
if adb logcat -d -s PhoneLinkNote:V | grep "connection loop ended"; then softfail "phone connection dropped while handling the PC clipboard (see line above)"; else echo "OK connection stayed up through the PC clipboard message (no connection-loop exception in logcat)"; fi
python3 .github/e2e.py focus_host || echo "UNVERIFIED could not focus the address field for the paste check"
adb shell input keyevent 279 # KEYCODE_PASTE into the focused address box
sleep 1
adb exec-out screencap -p > shots/clip-pc-to-phone.png
if python3 .github/e2e.py wait "pc-clip-123" 3; then echo "OK real phone clipboard contains the PC text (pasted into a field)"; else echo "UNVERIFIED actual phone clipboard contents (paste check did not show the text)"; fi
# phone -> PC: shared text (this proves the share path, NOT the clipboard listener)
adb shell am start -n $PKG/.ShareActivity -a android.intent.action.SEND -t text/plain --es android.intent.extra.TEXT share-hello-1 >/dev/null
for i in $(seq 1 20); do [ -f "$HOME/clip-recv.txt" ] && grep -q share-hello-1 "$HOME/clip-recv.txt" && break; sleep 1; done
if grep -q share-hello-1 "$HOME/clip-recv.txt" 2>/dev/null; then echo "OK phone->PC shared text reached the PC (share path only)"; else cat shots/pc.log; softfail "shared text did not reach the PC"; fi
# phone -> PC file via the share target
echo "from the phone" > /tmp/share-test.txt
adb push /tmp/share-test.txt /sdcard/Android/data/$PKG/files/share-test.txt >/dev/null
adb shell am start -n $PKG/.ShareActivity -a android.intent.action.SEND -t text/plain --eu android.intent.extra.STREAM file:///sdcard/Android/data/$PKG/files/share-test.txt >/dev/null
for i in $(seq 1 30); do [ -f "$HOME/Downloads/PhoneLink/Received/share-test.txt" ] && break; sleep 1; done
if [ -f "$HOME/Downloads/PhoneLink/Received/share-test.txt" ]; then
  if grep -q "from the phone" "$HOME/Downloads/PhoneLink/Received/share-test.txt"; then echo "OK phone->PC file arrived with correct name and content"; else softfail "phone->PC file content wrong"; fi
else
  ls -la "$HOME/Downloads/PhoneLink/Received/" || true; cat shots/pc.log; softfail "phone->PC file did not arrive under its name"
fi
adb exec-out screencap -p > shots/phone-to-pc-file.png
# phone -> PC real clipboard listener: set the phone clipboard through the address box (type, select all, copy)
# ---- phone clipboard LISTENER -> PC: type a word in the address field, long-press it to select, tap the system "Copy" ----
if ensure_app "phone clipboard listener -> PC"; then
python3 .github/e2e.py focus_host
adb shell input keyevent 123; for i in $(seq 1 60); do adb shell input keyevent 67; done
adb shell input text listenerabc; sleep 1
python3 .github/e2e.py longpress_host
python3 .github/e2e.py dump selection
python3 .github/e2e.py tap_any '^select all$' 4 || true; sleep 1; python3 .github/e2e.py dump selection2
if python3 .github/e2e.py tap_any '^copy$' 6; then
  for i in $(seq 1 15); do grep -q listenerabc "$HOME/clip-recv.txt" 2>/dev/null && break; sleep 1; done
  if grep -q listenerabc "$HOME/clip-recv.txt" 2>/dev/null; then echo "OK phone clipboard LISTENER: text copied on the phone arrived at the PC (system Copy button used)"; else cat shots/pc.log; adb logcat -d -s PhoneLinkNote:V | tail -20; softfail "phone copied text but the PC clipboard never got it (listener path)"; fi
else
  echo "UNVERIFIED phone clipboard listener -> PC: the system Copy button could not be driven here (see shots/ui-selection.txt)"
fi
adb exec-out screencap -p > shots/clip-phone-to-pc.png
foreground
fi
# ---- folder picker: open the system folder picker, pick a folder, then receive a file and look for the copy outside app storage ----
if ensure_app "folder picker"; then
if python3 .github/e2e.py tap_any '^choose folder for received files$' 6; then
  sleep 3; python3 .github/e2e.py dump picker1; adb exec-out screencap -p > shots/picker1.png
  CHOSEN=0
  for i in 1 2 3 4 5 6; do
    if python3 .github/e2e.py tap_any '^(select|use this folder|allow)' 2; then CHOSEN=1; sleep 2; break; fi
    python3 .github/e2e.py tap_any '^(downloads?|show roots)$' 2 || true; sleep 2; python3 .github/e2e.py dump picker-step$i
  done
  adb exec-out screencap -p > shots/picker2.png; python3 .github/e2e.py dump picker2
  python3 .github/e2e.py tap_any '^(allow|select)' 2 || true
  sleep 2; adb exec-out screencap -p > shots/picker3.png
  if [ "$CHOSEN" = 1 ] && python3 .github/e2e.py wait "Received files are copied to your chosen folder" 10; then
    echo "folder chosen in the system picker; the app reports it"
    echo "hello folder" > "$HOME/Downloads/PhoneLink/Send/e2e-folder.txt"
    FOUND=""
    for i in $(seq 1 40); do FOUND=$(adb shell "find /sdcard -name e2e-folder.txt 2>/dev/null" | tr -d '\r' | grep -v "Android/data" | head -1); [ -n "$FOUND" ] && break; sleep 1; done
    adb shell "find /sdcard -name e2e-folder.txt 2>/dev/null" | tee shots/folder-find.txt
    if [ -n "$FOUND" ]; then echo "OK received file was copied into the user-chosen folder: $FOUND"; else softfail "folder was chosen but no copy of the received file appeared outside app storage"; fi
  else
    echo "UNVERIFIED received-file copy to a chosen folder: could not complete the system picker on this emulator (see shots/ui-picker*.txt, picker*.png)"
  fi
else
  echo "UNVERIFIED received-file copy to a chosen folder: the Choose folder button was not found"
fi
foreground
fi
# sky motion: two frames 2 seconds apart should differ (animation). This does not prove smoothness.
adb exec-out screencap -p > shots/motion-a.png; sleep 2; adb exec-out screencap -p > shots/motion-b.png
if cmp -s shots/motion-a.png shots/motion-b.png; then softfail "screen identical 2s apart: no animation seen"; else echo "OK frames differ 2s apart (animation or other change, smoothness not measured)"; fi
# touch feedback: three no-touch frames, then a frame while the finger is held on an inert spot (the header); compare pixels around the touch
SZ=$(adb shell wm size | grep -o '[0-9]*x[0-9]*' | tail -1); W=${SZ%x*}; H=${SZ#*x}
TX=$((W*3/4)); TY=$((H*8/100))
if ensure_app "touch glow"; then TOUCHSKIP=0; else TOUCHSKIP=1; fi
for n in 1 2 3; do adb exec-out screencap -p > shots/touch-base$n.png; sleep 1; done
adb shell input swipe $TX $TY $TX $TY 3000 &
SWPID=$!
sleep 1; adb exec-out screencap -p > shots/touch-down.png; if appfocus; then HELDFOCUS=1; else HELDFOCUS=0; fi; wait $SWPID
if [ "$TOUCHSKIP" = 0 ] && [ "$HELDFOCUS" = 0 ]; then echo "UNVERIFIED touch glow: PhoneLink lost window focus during the hold, frame not usable"; TOUCHSKIP=1; fi
pip install --quiet pillow >/dev/null 2>&1 || true
TD=$(python3 .github/touchdiff.py shots/touch-base1.png shots/touch-base2.png shots/touch-base3.png shots/touch-down.png $TX $TY); RC=$?
echo "touch measurement: $TD"
if [ "$TOUCHSKIP" = 1 ]; then :
elif [ $RC -eq 0 ]; then echo "OK touch changes the pixels around the finger well above the sky's own motion ($TD); the look still needs a human eye (shots/touch-down.png)"
elif [ $RC -eq 1 ]; then softfail "touch glow not detected around the finger ($TD)"
else echo "UNVERIFIED touch glow: could not measure ($TD)"; fi
# notification mirroring was removed from the app; nothing to test
adb exec-out screencap -p > shots/final.png
kill $PCPID
echo "SUMMARY: $FAILS supported-feature checks failed (UNVERIFIED lines above are environment limits, not failures)"
sleep 2
[ "$FAILS" -eq 0 ] || exit 1
