# GIFs für die Webseite (site/gifs)

Aufnahme der echten App in tmux gegen den Test-SSH-Server und ein Demo-LLM,
Bild für Bild über Chromium (Playwright) gerendert und mit ffmpeg zum GIF gebaut.

```sh
go build -ldflags "-X github.com/db9979/sshtermy/internal/version.Version=1.0.96" -o /tmp/st/sshtermy ./cmd/sshtermy
(cd tools/dev/devsshd && go build -o /tmp/st/devsshd .)
PATH=$PWD/tools/dev/gifs/fakebin:$PATH /tmp/st/devsshd &   # docker, curl, systemctl … als Attrappen
cd tools/dev/gifs
DEMO_LANG=en python3 demollm.py &      # Port 11434, Antworten auf Englisch (de = Deutsch)
python3 prepare.py en                  # Master-Passwort demo1234, drei Verbindungen
# einmal von Hand mit web-01 verbinden und den Host-Key bestätigen
python3 scenes.py en start goal ghost palette
cp out/*.gif ../../../site/gifs/
```

Für Deutsch `demollm.py` mit `DEMO_LANG=de` neu starten und `prepare.py de` / `scenes.py de …`.
Braucht tmux, ffmpeg, Node mit Playwright und Python 3.
