import re, sys, html
PAL = ["#15161e","#f7768e","#9ece6a","#e0af68","#7aa2f7","#bb9af7","#7dcfff","#a9b1d6","#414868","#f7768e","#9ece6a","#e0af68","#7aa2f7","#bb9af7","#7dcfff","#c0caf5"]
def x256(n):
    if n < 16: return PAL[n]
    if n < 232:
        n -= 16; r, g, b = n//36, (n//6)%6, n%6
        f = lambda v: 0 if v == 0 else 55 + v*40
        return "#%02x%02x%02x" % (f(r), f(g), f(b))
    v = 8 + (n-232)*10; return "#%02x%02x%02x" % (v, v, v)
DFG, DBG = "#c0caf5", "#1a1b26"
def conv(text):
    out = []; st = {"fg": None, "bg": None, "b": False, "i": False, "u": False, "rev": False, "dim": False}
    for line in text.split("\n"):
        parts = re.split(r"(\x1b\[[0-9;:]*m)", line); spans = []
        for p in parts:
            if p.startswith("\x1b["):
                codes = p[2:-1].replace(":", ";").split(";") if p[2:-1] else ["0"]; i = 0
                while i < len(codes):
                    c = int(codes[i] or 0)
                    if c == 0: st = {"fg": None, "bg": None, "b": False, "i": False, "u": False, "rev": False, "dim": False}
                    elif c == 1: st["b"] = True
                    elif c == 2: st["dim"] = True
                    elif c == 3: st["i"] = True
                    elif c == 4: st["u"] = True
                    elif c == 7: st["rev"] = True
                    elif c == 22: st["b"] = st["dim"] = False
                    elif c == 23: st["i"] = False
                    elif c == 24: st["u"] = False
                    elif c == 27: st["rev"] = False
                    elif 30 <= c <= 37: st["fg"] = PAL[c-30]
                    elif 90 <= c <= 97: st["fg"] = PAL[c-90+8]
                    elif 40 <= c <= 47: st["bg"] = PAL[c-40]
                    elif 100 <= c <= 107: st["bg"] = PAL[c-100+8]
                    elif c == 39: st["fg"] = None
                    elif c == 49: st["bg"] = None
                    elif c in (38, 48):
                        key = "fg" if c == 38 else "bg"
                        if codes[i+1] == "5": st[key] = x256(int(codes[i+2])); i += 2
                        elif codes[i+1] == "2": st[key] = "#%02x%02x%02x" % tuple(int(x) for x in codes[i+2:i+5]); i += 4
                    i += 1
                continue
            if not p: continue
            fg, bg = st["fg"] or DFG, st["bg"]
            if st["rev"]: fg, bg = (bg or DBG), fg
            css = "color:%s;" % fg + ("background:%s;" % bg if bg else "")
            if st["b"]: css += "font-weight:700;"
            if st["i"]: css += "font-style:italic;"
            if st["u"]: css += "text-decoration:underline;"
            if st["dim"]: css += "opacity:.6;"
            spans.append('<span style="%s">%s</span>' % (css, html.escape(p)))
        out.append("".join(spans))
    return "\n".join(out)

def page(text, title):
    return """<!doctype html><html><head><meta charset="utf-8"><style>
body{margin:0;background:#0d1117;padding:0;font-family:'DejaVu Sans Mono',monospace}
.win{display:inline-block;border-radius:10px;overflow:hidden;border:1px solid #2a2e45}
.bar{background:#24283b;height:30px;display:flex;align-items:center;padding:0 12px;gap:7px;color:#a9b1d6;font:13px system-ui,sans-serif}
.d{width:11px;height:11px;border-radius:50%%}.t{margin-left:12px}
pre{margin:0;width:120ch;height:%dem;background:#1a1b26;color:#c0caf5;font-size:13px;line-height:1.25;padding:8px 10px;font-family:'DejaVu Sans Mono',monospace;overflow:hidden}
</style></head><body><div class="win"><div class="bar"><span class="d" style="background:#f7768e"></span><span class="d" style="background:#e0af68"></span><span class="d" style="background:#9ece6a"></span><span class="t">%s</span></div><pre>%s</pre></div></body></html>""" % (ROWS*1.25, html.escape(title), conv(text))
ROWS = 34
