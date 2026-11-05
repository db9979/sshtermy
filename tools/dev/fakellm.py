import json, re, time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

STEPS = [
    {"befehl": "docker --version", "grund": "Prüfen, ob Docker schon installiert ist.", "plan": ["Docker prüfen", "Docker installieren", "Nginx-Container starten", "Reverse Proxy konfigurieren", "Test mit curl"]},
    {"befehl": "sudo apt-get install -y docker.io docker-compose-v2", "grund": "Docker Engine und Compose aus den Ubuntu-Paketen installieren.", "plan": ["Nginx-Container starten", "Reverse Proxy konfigurieren", "Test mit curl"]},
    {"befehl": "docker run -d --name proxy -p 80:80 -v ~/proxy/nginx.conf:/etc/nginx/conf.d/default.conf:ro nginx:stable", "grund": "Nginx als Reverse Proxy mit eigener Konfiguration starten.", "plan": ["Test mit curl"]},
    {"fertig": True, "grund": "Docker läuft und Nginx leitet Port 80 an die App auf :3000 weiter."},
]

class H(BaseHTTPRequestHandler):
    def log_message(self, *a): pass
    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        sysm = body["messages"][0]["content"]; user = body["messages"][-1]["content"]
        if "vervollständigst" in sysm:
            typed = re.search(r"Bisher getippt: (.*)", user).group(1)
            table = {"docker": "docker ps -a --format 'table {{.Names}}\\t{{.Status}}'", "sys": "systemctl status nginx --no-pager",
                     "jour": "journalctl -u nginx -n 50 --no-pager", "df": "df -h /", "git": "git status -sb"}
            out = typed
            for k, v in table.items():
                if v.startswith(typed): out = v
            text = json.dumps({"zeile": out})
        elif "2 bis 4" in sysm:
            text = json.dumps([{"befehl": "docker ps", "grund": "laufende Container"}, {"befehl": "free -h", "grund": "Speicher"},
                               {"befehl": "df -h", "grund": "freier Platz"}, {"befehl": "docker system prune -a", "grund": "Platz schaffen"}])
        elif "Ziel" in sysm:
            n = len(re.findall(r"^\d+\. .*\((ausgeführt|vom Menschen übersprungen)\)$", user, re.M))
            text = json.dumps(STEPS[min(n, len(STEPS)-1)])
        else:
            text = "Nginx startet nicht, weil Port 80 schon belegt ist (siehe `bind() failed`). Prüfe, wer lauscht:\n\n```sh\nsudo ss -tlnp | grep :80\n```\n\nIst es ein alter Apache, stoppe ihn:\n\n```sh\nsudo systemctl stop apache2\n```\n"
        if body.get("stream"):
            self.send_response(200); self.send_header("Content-Type", "text/event-stream"); self.end_headers()
            for i in range(0, len(text), 12):
                self.wfile.write(("data: " + json.dumps({"choices": [{"delta": {"content": text[i:i+12]}}]}) + "\n\n").encode()); self.wfile.flush(); time.sleep(0.02)
            self.wfile.write(b"data: [DONE]\n\n"); return
        time.sleep(0.2)
        out = json.dumps({"choices": [{"message": {"role": "assistant", "content": text}}]}).encode()
        self.send_response(200); self.send_header("Content-Type", "application/json"); self.send_header("Content-Length", str(len(out))); self.end_headers(); self.wfile.write(out)

ThreadingHTTPServer(("127.0.0.1", 11434), H).serve_forever()
