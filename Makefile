# sshTermy Build. Version = MAJOR.MINOR aus VERSION + Anzahl Commits.
VERSION := $(shell cat VERSION).$(shell git rev-list --count HEAD 2>/dev/null || echo 0)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
PKG     := github.com/db9979/sshtermy/internal/version
LDFLAGS := -s -w -X $(PKG).Version=$(VERSION) -X $(PKG).Commit=$(COMMIT) -X $(PKG).Date=$(DATE) -X $(PKG).UpdatePubKey=$(UPDATE_PUBKEY)
TARGETS := windows/arm64 windows/386 windows/amd64 darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 linux/arm linux/riscv64

.PHONY: build test bench loadtest dist clean run golden handout fuzz vuln icon zusehen
build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/sshtermy ./cmd/sshtermy

test:
	go vet ./... && go test ./...
	cd third_party/vt && go test ./...
	cd third_party/ansi && go test ./...

# Leistung: Terminal-Durchsatz, Speicher pro Tab, Manager mit 500 Verbindungen
bench:
	go test -run '^$$' -bench . -benchtime 2s ./internal/termemu ./internal/ui ./internal/store

# Lasttest mit 1000 Hosts (Manager, Palette, Online-Prüfung, Broadcast, Sync, MCP)
loadtest:
	SSHTERMY_LOAD=1 go test -run TestLoad1000 -v -count=1 ./internal/ui

# Fuzz-Tests (Parser für Daten von fremden Geräten), je FUZZTIME lang
FUZZTIME ?= 20s
fuzz:
	FUZZTIME=$(FUZZTIME) tools/fuzz/run.sh

# Bekannte Sicherheitslücken in Go und den Bibliotheken
vuln:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

# Bildschirm-Tests (internal/ui/testdata/golden) nach gewollten Änderungen neu schreiben
golden:
	go test ./internal/ui -run TestGolden -update

# Kurzreferenz zum Ausdrucken (site/handout/*.pdf) + Fingerabdruck der Tasten, braucht Node + Playwright
handout:
	node tools/handout/render.mjs
	HANDOUT_STAMP=1 go test ./internal/ui -run TestHandoutKeysCurrent -count=1

# Zusehen im Browser: WebAssembly aus cmd/zusehen-wasm (+ passendes wasm_exec.js), eingecheckt
zusehen:
	GOOS=js GOARCH=wasm CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o internal/webwatch/assets/zusehen.wasm ./cmd/zusehen-wasm
	gzip -9 -n -f internal/webwatch/assets/zusehen.wasm
	chmod 644 internal/webwatch/assets/zusehen.wasm.gz
	install -m 644 "$$(go env GOROOT)/lib/wasm/wasm_exec.js" internal/webwatch/assets/wasm_exec.js
	ZUSEHEN_STAMP=1 go test ./internal/webwatch -run TestWasmCurrent -count=1

# App-Icon neu erzeugen (assets/icon, site/*.png, Windows-.syso), braucht Python+Pillow und Node+Playwright
icon:
	python3 tools/icon/gen.py
	node tools/icon/render.mjs
	python3 tools/icon/pack.py
	go run ./tools/icon/winres

dist: test
	@mkdir -p dist
	@for t in $(TARGETS); do \
	  os=$${t%/*}; arch=$${t#*/}; ext=""; [ $$os = windows ] && ext=".exe"; \
	  echo "-> $$os/$$arch"; \
	  CGO_ENABLED=0 GOARM=7 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "$(LDFLAGS)" -o dist/sshtermy_$${os}_$${arch}$$ext ./cmd/sshtermy || exit 1; \
	done
	cd dist && sha256sum sshtermy_* > checksums.txt

run: build
	./bin/sshtermy

clean:
	rm -rf bin dist
