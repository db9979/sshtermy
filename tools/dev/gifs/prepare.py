import sys, shutil
from record import Rec
lang = sys.argv[1]; home = f"/tmp/st/home-{lang}"
shutil.rmtree(home, ignore_errors=True)
r = Rec(lang, home, "--no-intro")
r.wait(2); r.type("demo1234", 0.02); r.k("Tab"); r.type("demo1234", 0.02); r.k("Enter", d=2); r.k("Escape", d=0.8)
for alias, host, user, tags in [("web-01", "127.0.0.1", "dominik", "web"), ("db-01", "127.0.0.2", "postgres", "db"), ("spark", "127.0.0.3", "dominik", "lab")]:
    r.k("n", d=1); r.type(alias, 0.01); r.k("Tab", "Tab"); r.type(host, 0.01); r.k("Tab"); r.k("BSpace", "BSpace", d=0.2); r.type("2222", 0.01); r.k("Tab")
    r.type(user, 0.01); r.k("Tab"); r.k("Right", "Right", d=0.3); r.k("Tab"); r.type("secret", 0.01); r.k("Tab", "Tab", d=0.3); r.type(tags, 0.01); r.k("C-s", d=1)
print(r.screen())
r.stop = True; import subprocess; subprocess.run(["tmux", "kill-session", "-t", "g"])
