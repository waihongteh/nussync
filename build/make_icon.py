"""Render the NUSSync app icon.

Draws an indigo (#5B5BD6) rounded square with a white sync arc and a bold "N",
then writes build/appicon.png (256x256) and build/windows/icon.ico (multi-size).
Wails picks up appicon.png at build time; the .ico is written directly so the
exe icon is correct even when the toolchain skips regeneration.

    python build/make_icon.py
"""

from __future__ import annotations

import math
import os

from PIL import Image, ImageDraw, ImageFont

INDIGO = (0x5B, 0x5B, 0xD6, 255)
INDIGO_DARK = (0x45, 0x45, 0xB8, 255)
WHITE = (255, 255, 255, 255)

HERE = os.path.dirname(os.path.abspath(__file__))
PNG_PATH = os.path.join(HERE, "appicon.png")
ICO_PATH = os.path.join(HERE, "windows", "icon.ico")

# Draw at 4x and downsample for clean edges.
SCALE = 4
SIZE = 256 * SCALE


def load_font(px: int) -> ImageFont.FreeTypeFont:
    for name in ("segoeuib.ttf", "arialbd.ttf", "DejaVuSans-Bold.ttf"):
        try:
            return ImageFont.truetype(name, px)
        except OSError:
            continue
    return ImageFont.load_default()


def arrow_head(d: ImageDraw.ImageDraw, cx: float, cy: float, angle: float, r: float) -> None:
    """Filled triangle centred at (cx, cy), pointing along `angle` radians."""
    pts = []
    for offset in (0.0, 2.4, -2.4):
        a = angle + offset
        pts.append((cx + r * math.cos(a), cy + r * math.sin(a)))
    d.polygon(pts, fill=WHITE)


def render() -> Image.Image:
    img = Image.new("RGBA", (SIZE, SIZE), (0, 0, 0, 0))
    d = ImageDraw.Draw(img)

    # Rounded square body with a subtle darker rim.
    radius = int(SIZE * 0.235)
    d.rounded_rectangle([0, 0, SIZE - 1, SIZE - 1], radius=radius, fill=INDIGO)
    d.rounded_rectangle(
        [0, 0, SIZE - 1, SIZE - 1], radius=radius, outline=INDIGO_DARK, width=int(SIZE * 0.012)
    )

    # Sync arc: two open arcs with arrow heads, framing the letter.
    pad = SIZE * 0.155
    box = [pad, pad, SIZE - pad, SIZE - pad]
    width = int(SIZE * 0.055)
    d.arc(box, start=138, end=318, fill=WHITE, width=width)
    d.arc(box, start=-42, end=138, fill=WHITE, width=width)

    r = (SIZE - 2 * pad) / 2.0
    cx = cy = SIZE / 2.0
    head = SIZE * 0.062
    for deg, point in ((138, 138 + 90), (318, 318 + 90)):
        a = math.radians(deg)
        arrow_head(d, cx + r * math.cos(a), cy + r * math.sin(a), math.radians(point), head)

    # "N" glyph.
    font = load_font(int(SIZE * 0.44))
    box = d.textbbox((0, 0), "N", font=font)
    d.text(
        (cx - (box[0] + box[2]) / 2.0, cy - (box[1] + box[3]) / 2.0),
        "N",
        font=font,
        fill=WHITE,
    )

    return img.resize((256, 256), Image.LANCZOS)


def main() -> None:
    icon = render()
    icon.save(PNG_PATH, "PNG")
    os.makedirs(os.path.dirname(ICO_PATH), exist_ok=True)
    icon.save(ICO_PATH, "ICO", sizes=[(16, 16), (24, 24), (32, 32), (48, 48), (64, 64), (128, 128), (256, 256)])
    print("wrote", PNG_PATH)
    print("wrote", ICO_PATH)


if __name__ == "__main__":
    main()
