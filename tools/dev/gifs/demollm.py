# Demo-LLM für die GIFs (Englisch/Deutsch über DEMO_LANG).
import json, re, time, os
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
EN = os.environ.get("DEMO_LANG", "en") == "en"
T = (lambda en, de: en) if EN else (lambda en, de: de)
STEPS = [
    {"befehl": "docker --version", "grund": T("Check whether Docker is already installed.", "Prüfen, ob Docker schon installiert ist."),
     "plan": [T("Check Docker", "Docker prüfen"), T("Start nginx container", "Nginx-Container starten"), T("Test with curl", "Test mit curl")]},
    {"befehl": "docker run -d --name proxy -p 80:80 -v ~/proxy/nginx.conf:/etc/nginx/conf.d/default.conf:ro nginx:stable",
     "grund": T("Start nginx as reverse proxy with its own config.", "Nginx als Reverse Proxy mit eigener Konfiguration starten."),
     "plan": [T("Start nginx container", "Nginx-Container starten"), T("Test with curl", "Test mit curl")]},
    {"befehl": "curl -sI http://localhost", "grund": T("Check that port 80 answers through the proxy.", "Prüfen, ob Port 80 über den Proxy antwortet."),
     "plan": [T("Test with curl", "Test mit curl")]},
    {"fertig": True, "grund": T("nginx is running and forwards port 80 to the app on :3000.", "Nginx läuft und leitet Port 80 an die App auf :3000 weiter.")},
]
ANSWER = T(
    "nginx does not start because port 80 is already in use (`bind() failed`). Check who is listening:\n\n```sh\nsudo ss -tlnp | grep :80\n```\n\nIf it is an old Apache, stop it:\n\n```sh\nsudo systemctl stop apache2\n```\n",
    "Nginx startet nicht, weil Port 80 schon belegt ist (`bind() failed`). Prüfe, wer lauscht:\n\n```sh\nsudo ss -tlnp | grep :80\n```\n\nIst es ein alter Apache, stoppe ihn:\n\n```sh\nsudo systemctl stop apache2\n```\n")

DISK = T(
    "`/` is 4% full, so the space is likely used by Docker images or logs. Find the biggest folders:\n\n```sh\nsudo du -xh / --max-depth=2 | sort -h | tail -15\n```\n\nOld images and stopped containers:\n\n```sh\ndocker system df\n```\n",
    "`/` ist zu 4 % belegt, der Platz steckt also eher in Docker-Images oder Logs. Die größten Ordner finden:\n\n```sh\nsudo du -xh / --max-depth=2 | sort -h | tail -15\n```\n\nAlte Images und gestoppte Container:\n\n```sh\ndocker system df\n```\n")

class H(BaseHTTPRequestHandler):
    def log_message(self, *a): pass
    def do_GET(self):
        if self.path.rstrip("/").endswith("/models"):
            out = json.dumps({"object": "list", "data": [{"id": m, "object": "model"} for m in ["qwen3-coder:30b", "gpt-oss:20b"]]}).encode()
            self.send_response(200); self.send_header("Content-Type", "application/json"); self.send_header("Content-Length", str(len(out))); self.end_headers(); self.wfile.write(out)
        else:
            self.send_response(404); self.end_headers()
    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        sysm = body["messages"][0]["content"]; user = body["messages"][-1]["content"]
        if "vervollständigst" in sysm:
            typed = re.search(r"Bisher getippt: (.*)", user).group(1)
            table = ["docker ps --format 'table {{.Names}}\\t{{.Status}}'", "systemctl status nginx --no-pager", "journalctl -u nginx -n 50 --no-pager", "free -h"]
            out = typed
            for v in table:
                if typed and v.startswith(typed): out = v; break
            text = json.dumps({"zeile": out})
        elif "2 bis 4" in sysm:
            text = json.dumps([{"befehl": "journalctl -u nginx -n 50 --no-pager", "grund": T("last log lines", "letzte Logzeilen")},
                               {"befehl": "sudo ss -tlnp | grep :80", "grund": T("who uses port 80", "wer belegt Port 80")},
                               {"befehl": "sudo nginx -t", "grund": T("check config", "Konfiguration prüfen")}])
        elif "Ziel" in sysm:
            n = len(re.findall(r"^\d+\. .*\((ausgeführt[^)]*|vom Menschen übersprungen)\)$", user, re.M))
            text = json.dumps(STEPS[min(n, len(STEPS)-1)])
        elif sysm.startswith("Antworte nur mit"):
            text = "OK"
        elif "disk" in user or "Platte" in user:
            text = DISK
        else:
            text = ANSWER
        if body.get("stream"):
            self.send_response(200); self.send_header("Content-Type", "text/event-stream"); self.end_headers()
            for i in range(0, len(text), 6):
                self.wfile.write(("data: " + json.dumps({"choices": [{"delta": {"content": text[i:i+6]}}]}) + "\n\n").encode()); self.wfile.flush(); time.sleep(0.03)
            self.wfile.write(b"data: [DONE]\n\n"); return
        time.sleep(0.25)
        out = json.dumps({"choices": [{"message": {"role": "assistant", "content": text}}]}).encode()
        self.send_response(200); self.send_header("Content-Type", "application/json"); self.send_header("Content-Length", str(len(out))); self.end_headers(); self.wfile.write(out)

ThreadingHTTPServer(("127.0.0.1", 11434), H).serve_forever()
