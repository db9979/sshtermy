#!/bin/sh
# Alle Fuzz-Tests nacheinander, je FUZZTIME lang (Standard 20s).
#   tools/fuzz/run.sh            oder   make fuzz FUZZTIME=5m
# Ein Fund landet in <paket>/testdata/fuzz/<Test>/… und läuft danach bei
# jedem normalen go test mit (Datei einchecken, Fehler beheben).
set -eu
T="${FUZZTIME:-20s}"
for pkg in $(grep -rl --include='*_test.go' '^func Fuzz' internal | xargs -n1 dirname | sort -u); do
	for fn in $(grep -h '^func Fuzz' "$pkg"/*_test.go | sed 's/func \(Fuzz[A-Za-z0-9_]*\).*/\1/'); do
		echo "== ./$pkg $fn ($T)"
		go test "./$pkg" -run '^$' -fuzz "^$fn\$" -fuzztime "$T"
	done
done
