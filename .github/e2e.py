#!/usr/bin/env python3
"""UI helpers for the emulator end-to-end test: find things on the phone screen via uiautomator."""
import re, subprocess, sys, time
import xml.etree.ElementTree as ET

def adb(*a):
    return subprocess.run(["adb", *a], capture_output=True, text=True).stdout

def dump():
    adb("shell", "uiautomator", "dump", "/sdcard/u.xml")
    adb("pull", "/sdcard/u.xml", "shots/u.xml")
    return ET.parse("shots/u.xml").getroot()

def center(node):
    x1, y1, x2, y2 = map(int, re.findall(r"\d+", node.get("bounds")))
    return (x1 + x2) // 2, (y1 + y2) // 2

def find(root, text=None, cls=None):
    for n in root.iter("node"):
        if text is not None and n.get("text") == text:
            return n
        if cls is not None and n.get("class") == cls:
            return n
    return None

def all_text(root):
    return [n.get("text") for n in root.iter("node") if n.get("text")]

def tap(text=None, cls=None, tries=10):
    for _ in range(tries):
        n = find(dump(), text, cls)
        if n is not None:
            x, y = center(n)
            adb("shell", "input", "tap", str(x), str(y))
            return True
        time.sleep(1)
    return False

def wait_text(pred, secs=40):
    end = time.time() + secs
    last = []
    while time.time() < end:
        last = all_text(dump())
        r = pred(last)
        if r:
            return r
        time.sleep(1)
    print("TIMEOUT; screen text was:", last)
    return None

if __name__ == "__main__":
    cmd = sys.argv[1]
    if cmd == "type_host":
        if not tap(cls="android.widget.EditText"): sys.exit("no address field")
        adb("shell", "input", "text", sys.argv[2])
        adb("shell", "input", "keyevent", "4")  # close keyboard
        time.sleep(1)
    elif cmd == "tap":
        if not tap(text=sys.argv[2]): sys.exit("no button " + sys.argv[2])
    elif cmd == "code":
        c = wait_text(lambda t: next((x for x in t if re.fullmatch(r"\d{6}", x)), None))
        if not c: sys.exit("phone never showed a pairing code")
        print(c)
    elif cmd == "wait":
        want = sys.argv[2]
        r = wait_text(lambda t: any(want in x for x in t), int(sys.argv[3]) if len(sys.argv) > 3 else 40)
        if not r: sys.exit("never saw: " + want)
