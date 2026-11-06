import subprocess as sp, time, threading, sys, os, shutil, json
sys.path.insert(0, os.path.dirname(__file__))
import ansiconv
S = os.path.dirname(os.path.abspath(__file__))
BIN = os.environ.get("SSHTERMY_BIN", "/tmp/st/sshtermy")
OUT = os.environ.get("GIF_OUT", os.path.join(S, "out"))
COLS, ROWS = 120, 34
ansiconv.ROWS = ROWS

def tm(*a): return sp.run(["tmux", *a], capture_output=True, text=True).stdout

class Rec:
    def __init__(self, lang, home, args=""):
        self.lang = lang
        tm("kill-session", "-t", "g")
        loc = "en_US.UTF-8" if lang == "en" else "de_DE.UTF-8"
        tm("new-session", "-d", "-s", "g", "-x", str(COLS), "-y", str(ROWS),
           f"SSHTERMY_HOME={home} LANG={loc} LC_ALL={loc} TERM=xterm-256color {BIN} {args}")
        self.frames, self.on, self.stop = [], False, False
        self.t = threading.Thread(target=self.loop, daemon=True); self.t.start()
    def loop(self):
        while not self.stop:
            if self.on:
                s = tm("capture-pane", "-e", "-p", "-t", "g")
                if not self.frames or self.frames[-1][1] != s:
                    self.frames.append((time.time(), s))
            time.sleep(0.08)
    def k(self, *keys, d=0.5):
        for key in keys: tm("send-keys", "-t", "g", key)
        time.sleep(d)
    def type(self, text, d=0.07, after=0.4):
        for ch in text:
            tm("send-keys", "-t", "g", "-l", ch); time.sleep(d)
        time.sleep(after)
    def wait(self, s): time.sleep(s)
    def screen(self): return tm("capture-pane", "-p", "-t", "g")
    def until(self, text, timeout=10):
        end = time.time() + timeout
        while time.time() < end:
            if text in self.screen(): return True
            time.sleep(0.1)
        print("TIMEOUT waiting for", repr(text)); print(self.screen()); return False
    def finish(self, name, hold=3.0):
        time.sleep(0.3); self.stop = True; self.t.join()
        tm("kill-session", "-t", "g")
        render(self.frames, name, hold)

def render(frames, name, hold):
    d = os.path.join(OUT, "frames", name); shutil.rmtree(d, ignore_errors=True); os.makedirs(d)
    lines = []
    for i, (ts, s) in enumerate(frames):
        open(f"{d}/{i:04d}.html", "w", encoding="utf-8").write(ansiconv.page(s, "sshTermy"))
        dur = (frames[i+1][0] - ts) if i + 1 < len(frames) else hold
        lines += [f"file '{d}/{i:04d}.png'", f"duration {max(dur, 0.04):.3f}"]
    sp.run(["node", os.path.join(S, "shot.js"), d, str(len(frames))], check=True)
    lines.append(f"file '{d}/{len(frames)-1:04d}.png'")
    open(f"{d}/list.txt", "w").write("\n".join(lines) + "\n")
    out = os.path.join(OUT, name + ".gif"); os.makedirs(os.path.dirname(out), exist_ok=True)
    sp.run(["ffmpeg", "-y", "-loglevel", "error", "-f", "concat", "-safe", "0", "-i", f"{d}/list.txt",
            "-vf", "split[a][b];[a]palettegen=max_colors=128:stats_mode=full[p];[b][p]paletteuse=dither=none", "-loop", "0", out], check=True)
    print(name, len(frames), "frames", os.path.getsize(out) // 1024, "KB")
