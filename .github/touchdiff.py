#!/usr/bin/env python3
"""touchdiff.py BASE1 BASE2 BASE3 TOUCH X Y : mean abs pixel difference in a box around (X,Y).
Noise = largest difference between consecutive no-touch frames (the sky moves by itself).
Prints 'noise=.. touch=..' and exits 0 when touch > 3*noise + 1.0, 1 when not, 2 when it cannot measure."""
import sys
try:
    from PIL import Image, ImageChops
except Exception as e:
    print("cannot measure: PIL missing", e); sys.exit(2)
b1, b2, b3, t, x, y = sys.argv[1:7]; x = int(x); y = int(y)
def load(p): return Image.open(p).convert("RGB")
def box(im):
    w, h = im.size
    return im.crop((max(0, x - 50), max(0, y - 50), min(w, x + 50), min(h, y + 50)))
def md(a, b):
    d = ImageChops.difference(box(a), box(b)); px = list(d.getdata())
    return sum(sum(p) for p in px) / (len(px) * 3.0)
f = [load(p) for p in (b1, b2, b3, t)]
noise = max(md(f[0], f[1]), md(f[1], f[2]))
touch = md(f[2], f[3])
print("noise=%.2f touch=%.2f" % (noise, touch))
sys.exit(0 if touch > 3 * noise + 1.0 else 1)
