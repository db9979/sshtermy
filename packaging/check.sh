#!/bin/sh
# CI-Prüfung der Paket-Skripte mit Dummy-Dateien und der Webseite (Links,
# Bilder). Lokal: packaging/check.sh
set -eu
t="$(mktemp -d)"
trap 'rm -rf "$t"' EXIT INT TERM
mkdir -p "$t/dist"
for n in windows_arm64.exe windows_386.exe windows_amd64.exe darwin_arm64 darwin_amd64 linux_amd64 linux_arm64 linux_arm linux_riscv64; do
	printf '#!/bin/sh\necho "sshTermy v1.0.0"\n' >"$t/dist/sshtermy_$n"
done
(cd "$t/dist" && sha256sum sshtermy_* >checksums.txt)

packaging/deb.sh 1.0.0 amd64 "$t/dist/sshtermy_linux_amd64" "$t/apt" >/dev/null
dpkg-deb -f "$t/apt/sshtermy_1.0.0_amd64.deb" Package Version Architecture
packaging/apt-repo.sh "$t/apt"
packaging/deb.sh 1.0.0 armhf "$t/dist/sshtermy_linux_arm" "$t/apt" >/dev/null
grep -q "^Filename: ./sshtermy_1.0.0_amd64.deb" "$t/apt/Packages"

packaging/formula.sh 1.0.0 "$t/dist/checksums.txt" >"$t/sshtermy.rb"
ruby -c "$t/sshtermy.rb"
[ "$(grep -c 'sha256 "[0-9a-f]\{64\}"' "$t/sshtermy.rb")" = 4 ]

d="$(packaging/winget.sh 1.0.0 "$t/dist/checksums.txt" "$t/w")"
python3 - "$d" <<'PY'
import sys, glob, yaml
files = glob.glob(sys.argv[1] + "/*.yaml")
assert len(files) == 4, files
for f in files:
    m = yaml.safe_load(open(f, encoding="utf-8"))
    assert m["PackageIdentifier"] == "db9979.sshTermy" and m["PackageVersion"] == "1.0.0", f
    for i in m.get("Installers", []):
        assert len(i["InstallerSha256"]) == 64, i
print("winget ok:", len(files), "Manifeste")
PY

# Webseite: jede lokale href/src muss existieren (bilder/ kommt aus docs/bilder).
python3 - <<'PY'
import re, os, sys
bad = 0
for root, _, files in os.walk("site"):
    for f in files:
        if not f.endswith(".html"):
            continue
        p = os.path.join(root, f)
        for ref in re.findall(r'(?:href|src)="([^"#:?]+)"', open(p, encoding="utf-8").read()):
            if ref.startswith("//"):
                continue
            q = os.path.normpath(os.path.join(root, ref))
            if q.startswith("site/bilder/"):
                q = "docs/bilder/" + q[len("site/bilder/"):]
            if ref.endswith("/"):
                q = os.path.join(q, "index.html")
            if not os.path.exists(q):
                print("fehlt:", p, "->", ref); bad += 1
sys.exit(1 if bad else 0)
PY
echo "packaging ok"
