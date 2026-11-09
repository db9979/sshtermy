#!/usr/bin/env python3
"""PNGs → sshtermy.ico (Windows) und sshtermy.icns (macOS), braucht Pillow."""
import os
from PIL import Image

D = os.path.join(os.path.dirname(__file__), "..", "..", "assets", "icon")
img = lambda n: Image.open(os.path.join(D, n)).convert("RGBA")

# .ico: jede Größe aus ihrer eigenen PNG (16–64 kleine Fassung)
sizes = [16, 24, 32, 48, 64, 128, 256]
# Pillow nimmt nur Größen ≤ erstes Bild, darum die größte zuerst
first, rest = img(f"sshtermy-{sizes[-1]}.png"), [img(f"sshtermy-{s}.png") for s in sizes[:-1]]
first.save(os.path.join(D, "sshtermy.ico"), sizes=[(s, s) for s in sizes], append_images=rest)

# .icns: Mac-Fassung, Pillow erzeugt alle Retina-Stufen aus 1024
img("mac-1024.png").save(os.path.join(D, "sshtermy.icns"))
print("sshtermy.ico sshtermy.icns")

# Webseite (site/ wird so wie es ist auf GitHub Pages kopiert)
S = os.path.join(os.path.dirname(__file__), "..", "..", "site")
img("sshtermy-32.png").save(os.path.join(S, "favicon.png"))
img("sshtermy-64.png").save(os.path.join(S, "logo.png"))
img("sshtermy-1024.png").resize((180, 180), Image.LANCZOS).save(os.path.join(S, "apple-touch-icon.png"))
print("site/favicon.png site/logo.png site/apple-touch-icon.png")
