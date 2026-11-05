"""Writes the fixtures of internal/images tests. Run from this directory: python3 gen.py (needs Pillow with WebP).

Every fixture carries text a stripped file must not: secret-* in profiles, EXIF, XMP.
"""
import struct
from PIL import Image


def tag_desc(text):
    a = text.encode() + b"\0"
    return b"desc\0\0\0\0" + struct.pack(">I", len(a)) + a + b"\0" * 8 + b"\0\0\0" + b"\0" * 67


def tag_text(text):
    return b"text\0\0\0\0" + text.encode() + b"\0"


def xyz(x, y, z):
    f = lambda v: struct.pack(">i", round(v * 65536))
    return b"XYZ \0\0\0\0" + f(x) + f(y) + f(z)


def icc(space, tags):
    """A version 2 display profile: secret words in its header, desc, cprt and a private tag."""
    tags = [("desc", tag_desc("secret-icc-desc")), ("cprt", tag_text("secret-icc-cprt"))] + tags + [
        ("dmdd", tag_desc("secret-icc-model")), ("priv", b"zzzz\0\0\0\0secret-icc-private")]
    table = 128 + 4 + 12 * len(tags)
    data, entries, seen = b"", [], {}
    for sig, body in tags:
        if body in seen:
            entries.append((sig, seen[body], len(body)))
            continue
        while len(data) % 4:
            data += b"\0"
        seen[body] = table + len(data)
        entries.append((sig, table + len(data), len(body)))
        data += body
    while len(data) % 4:
        data += b"\0"
    size = table + len(data)
    head = struct.pack(">I", size) + b"SCRT" + bytes([2, 0x10, 0, 0]) + b"mntr" + space + b"XYZ "
    head += struct.pack(">6H", 2026, 11, 5, 12, 0, 0) + b"acsp" + b"SCRT" + b"\0\0\0\0" + b"SCRT" + b"MODL"
    head += b"\0" * 8 + b"\0\0\0\0" + xyz(0.9642, 1.0, 0.8249)[8:] + b"SCRT" + b"\x11" * 16 + b"\0" * 28
    assert len(head) == 128, len(head)
    out = head + struct.pack(">I", len(entries))
    for sig, off, n in entries:
        out += sig.encode() + struct.pack(">II", off, n)
    return out + data


curve = b"curv\0\0\0\0\0\0\0\x01\x02\x33\0\0"
rgb = icc(b"RGB ", [("wtpt", xyz(0.9642, 1.0, 0.8249)), ("rXYZ", xyz(0.4361, 0.2225, 0.0139)),
                    ("gXYZ", xyz(0.3851, 0.7169, 0.0971)), ("bXYZ", xyz(0.1431, 0.0606, 0.7141)),
                    ("rTRC", curve), ("gTRC", curve), ("bTRC", curve)])
gray = icc(b"GRAY", [("wtpt", xyz(0.9642, 1.0, 0.8249)), ("kTRC", curve)])
open("rgb.icc", "wb").write(rgb)
open("gray.icc", "wb").write(gray)


def exif(orient):
    desc = b"secret-webp-exif\0"
    ifd = struct.pack("<H", 2) + struct.pack("<HHII", 0x010E, 2, len(desc), 38) + struct.pack("<HHIHH", 0x0112, 3, 1, orient, 0) + b"\0\0\0\0"
    return b"Exif\0\0" + b"II*\0" + struct.pack("<I", 8) + ifd + desc


img = Image.new("RGBA", (12, 7), (220, 40, 40, 255))
for x in range(6, 12):
    for y in range(7):
        img.putpixel((x, y), (40, 60, 220, 128))
xmp = b'<x:xmpmeta xmlns:x="adobe:ns:meta/">secret-webp-xmp</x:xmpmeta>'
img.save("meta.webp", "WEBP", quality=90, exif=exif(1), xmp=xmp, icc_profile=rgb)
img.save("turned.webp", "WEBP", quality=90, exif=exif(6))
img.convert("RGB").save("plain.webp", "WEBP", lossless=True)
frames = [Image.new("RGB", (12, 7), c) for c in ((255, 0, 0), (0, 0, 255))]
frames[0].save("anim.webp", "WEBP", save_all=True, append_images=frames[1:], duration=100, loop=0)
