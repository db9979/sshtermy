import sys
from record import Rec
lang = sys.argv[1]; which = sys.argv[2:]
home = f"/tmp/st/home-{lang}"
L = lambda en, de: en if lang == "en" else de

def connected(args="--no-intro"):
    r = Rec(lang, home, args)
    r.wait(1.5); r.type("demo1234", 0.01); r.k("Enter", d=1.5)
    r.k("Down", "Down", "Down", d=0.3); r.k("Enter"); r.until("dgx-spark"); r.wait(1.5)
    return r

def start():
    r = Rec(lang, home, ""); r.on = True
    r.wait(4.5); r.type("demo1234", 0.09, 0.4); r.k("Enter", d=2)
    r.k("Down", d=0.6); r.k("Down", d=0.6); r.k("Down", d=1); r.k("Enter"); r.until("dgx-spark"); r.wait(1.5)
    r.type("free -h", 0.1, 0.8); r.k("Enter", d=2.5)
    r.finish("start-" + lang)

def goal():
    r = connected(); r.on = True; r.wait(0.8)
    r.k("F2", d=0.8); r.type(L("Set up nginx as reverse proxy for my app on :3000", "Nginx als Reverse Proxy für meine App auf :3000 einrichten"), 0.045, 0.6)
    r.k("Enter"); r.until("docker --version"); r.wait(2)
    r.k("Enter"); r.until("docker run"); r.wait(2.5)
    r.k("Enter"); r.until("curl -sI"); r.wait(2.5)
    r.k("Enter"); r.until(L("forwards port 80", "leitet Port 80"), 15); r.wait(1)
    r.finish("goal-" + lang, 4)

def ghost():
    r = connected(); r.on = True; r.wait(0.8)
    r.type("sys", 0.25, 1.6); r.k("Tab", d=1.2); r.k("Enter", d=3.5)
    r.k("M-0", d=5)
    r.k("1", d=1.5); r.k("Enter", d=3)
    r.finish("ghost-" + lang, 4)

def palette():
    r = connected(); r.on = True; r.wait(0.8)
    r.k("C-Space", d=2)
    r.type("tun", 0.25, 2)
    r.k("C-u", d=0.6)
    r.type(L("why is the disk full", "warum ist die Platte voll"), 0.06, 1.5)
    r.k("Tab"); r.until("docker system df", 10); r.wait(1)
    r.finish("palette-" + lang, 4)

for w in which: globals()[w]()
