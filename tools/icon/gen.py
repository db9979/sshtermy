#!/usr/bin/env python3
"""App-Icon „Phosphor“: schreibt die SVG-Vorlagen nach assets/icon/.

  sshtermy.svg        Mac-Fassung (Körper 824 px in 1024, Apple-Raster)
  sshtermy-full.svg   Windows/Linux/Web (Körper fast randlos)
  sshtermy-small.svg  16–64 px: ohne Regen und Zeilen, dickere Striche

Danach render.mjs (PNG, Playwright) und pack.py (.ico, .icns). Alles: make icon
"""
import os, random

OUT = os.path.join(os.path.dirname(__file__), "..", "..", "assets", "icon")
GREEN, DIM, HEAD = "#39ff14", "#1f9e10", "#b8ffb0"


def defs(glow):
    return f'''<defs>
<filter id="sh" x="-20%" y="-20%" width="140%" height="140%"><feDropShadow dx="0" dy="14" stdDeviation="18" flood-color="#000" flood-opacity=".45"/></filter>
<filter id="g" x="-50%" y="-50%" width="200%" height="200%"><feGaussianBlur stdDeviation="{glow}" result="b"/><feMerge><feMergeNode in="b"/><feMergeNode in="SourceGraphic"/></feMerge></filter>
<radialGradient id="crt" cx=".5" cy=".45" r=".7"><stop offset="0" stop-color="#0c2a0c"/><stop offset="1" stop-color="#020702"/></radialGradient>
<clipPath id="c"><rect x="100" y="100" width="824" height="824" rx="185"/></clipPath>
</defs>'''


def body(shadow=True):
    s = ' filter="url(#sh)"' if shadow else ""
    return (f'<g{s}><rect x="100" y="100" width="824" height="824" rx="185" fill="url(#crt)"/>'
            '<rect x="104" y="104" width="816" height="816" rx="181" fill="none" stroke="#1f5c1a" stroke-width="8"/></g>')


def rain():
    random.seed(7)
    out = []
    for col in range(13):
        x, top, n = 140 + col * 60, random.randint(110, 600), random.randint(5, 11)
        for k in range(n):
            y = top + k * 58
            if y > 900:
                break
            ch = random.choice("01ア>_$#ｼﾂﾃ7")
            fill = HEAD if k == n - 1 else DIM
            out.append(f'<text x="{x}" y="{y}" fill="{fill}" fill-opacity="{(k + 1) / n * .8:.2f}" '
                       f'font-family="DejaVu Sans Mono,monospace" font-size="48" font-weight="bold">{ch}</text>')
    return "".join(out)


def scanlines():
    return "".join(f'<rect x="100" y="{y}" width="824" height="3" fill="#000" fill-opacity=".35"/>' for y in range(104, 924, 9))


def prompt(w=80):
    return (f'<g filter="url(#g)" fill="{GREEN}"><path d="M262 430 L452 590 L262 750" fill="none" stroke="{GREEN}" '
            f'stroke-width="{w}" stroke-linecap="square" stroke-linejoin="miter"/>'
            f'<rect x="520" y="{782 - w}" width="250" height="{w}"/></g>')


def svg(inner, scale=1.0):
    t = f' transform="translate(512 512) scale({scale}) translate(-512 -512)"' if scale != 1 else ""
    return f'<svg xmlns="http://www.w3.org/2000/svg" width="1024" height="1024" viewBox="0 0 1024 1024"><g{t}>{inner}</g></svg>\n'


def big(shadow):
    return defs(18) + body(shadow) + f'<g clip-path="url(#c)">{rain()}<rect x="100" y="100" width="824" height="824" fill="url(#crt)" fill-opacity=".35"/>{prompt()}{scanlines()}</g>'
# klein: Körper randlos, Zeichen größer und kräftiger, damit 16 px lesbar bleibt
small = defs(10) + body(False) + ('<g clip-path="url(#c)"><g transform="translate(512 600) scale(1.25) translate(-516 -590)">'
                                  + prompt(110) + '</g></g>')

# randlose Fassung ohne Schatten (würde am Bildrand abgeschnitten)
for name, s in (("sshtermy.svg", svg(big(True))), ("sshtermy-full.svg", svg(big(False), 960 / 824)),
                ("sshtermy-small.svg", svg(small, 1020 / 824))):
    with open(os.path.join(OUT, name), "w") as f:
        f.write(s)
    print(name)
