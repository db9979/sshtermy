package offlineai

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func names(ns ...string) []asset {
	var out []asset
	for _, n := range ns {
		out = append(out, asset{Name: n, URL: "u/" + n})
	}
	return out
}

func TestPickServer(t *testing.T) {
	as := names("llama-b7000-bin-win-cpu-x64.zip", "llama-b7000-bin-win-cuda-12.4-x64.zip", "llama-b7000-bin-win-vulkan-x64.zip",
		"llama-b7000-bin-win-cpu-arm64.zip", "llama-b7000-bin-macos-arm64.zip", "llama-b7000-bin-macos-x64.zip",
		"llama-b7000-bin-ubuntu-x64.zip", "llama-b7000-bin-ubuntu-vulkan-x64.zip", "llama-b7000-bin-ubuntu-arm64.zip",
		"llama-b7000-xcframework.zip", "cudart-llama-bin-win-cuda-12.4-x64.zip")
	cases := map[string]string{
		"windows/amd64": "llama-b7000-bin-win-cpu-x64.zip",
		"windows/arm64": "llama-b7000-bin-win-cpu-arm64.zip",
		"darwin/arm64":  "llama-b7000-bin-macos-arm64.zip",
		"darwin/amd64":  "llama-b7000-bin-macos-x64.zip",
		"linux/amd64":   "llama-b7000-bin-ubuntu-x64.zip",
		"linux/arm64":   "llama-b7000-bin-ubuntu-arm64.zip",
	}
	for k, want := range cases {
		os_, arch, _ := strings.Cut(k, "/")
		got, ok := PickServer(os_, arch, as)
		if !ok || got.Name != want {
			t.Errorf("%s: %q, want %q", k, got.Name, want)
		}
	}
	if _, ok := PickServer("linux", "riscv64", as); ok {
		t.Error("riscv64 should have no build")
	}
	// umbenannte Pakete
	for k, n := range map[string]string{
		"darwin/arm64":  "llama-b8000-macos-arm64.tar.gz",
		"darwin/amd64":  "llama-b8000-bin-osx-x86_64.tgz",
		"linux/amd64":   "llama-b8000-bin-linux-x86_64.tar.gz",
		"windows/amd64": "llama-b8000-bin-win-cpu-x64.zip",
	} {
		os_, arch, _ := strings.Cut(k, "/")
		if got, ok := PickServer(os_, arch, names(n, "llama-b8000-bin-ubuntu-vulkan-x64.tar.gz")); !ok || got.Name != n {
			t.Errorf("%s: %q, want %q", k, got.Name, n)
		}
	}
	if got, ok := PickServer("darwin", "arm64", names("llama-b8000-bin-macos.zip")); !ok || got.Name != "llama-b8000-bin-macos.zip" {
		t.Errorf("Mac-Universal-Build: %q", got.Name)
	}
	if _, ok := PickServer("darwin", "arm64", names("llama-b8000-bin-macos-x64.zip")); ok {
		t.Error("arm64 must not take an x64 Mac build")
	}
	if _, ok := PickServer("linux", "arm64", names("llama-b1-bin-ubuntu-x64.zip")); ok {
		t.Error("arm64 must not take an x64 build")
	}
}

func TestPickModel(t *testing.T) {
	m, s, ok := PickModel(names("lxask-x86_64", "lxask-round4-Q4_K_M.gguf", "restrict-round4.gguf", "embed.gguf", "SHA256SUMS"))
	if !ok || m.Name != "lxask-round4-Q4_K_M.gguf" || s.Name != "SHA256SUMS" {
		t.Fatalf("%+v %+v %v", m, s, ok)
	}
	sums := ParseSums(strings.NewReader(strings.Repeat("a", 64) + "  lxask-round4-Q4_K_M.gguf\n" + strings.Repeat("b", 64) + " *embed.gguf\nbad line\n"))
	if sums["lxask-round4-Q4_K_M.gguf"] != strings.Repeat("a", 64) || sums["embed.gguf"] != strings.Repeat("b", 64) || len(sums) != 2 {
		t.Fatalf("sums %v", sums)
	}
}

func TestExtractZipAndFind(t *testing.T) {
	dir := t.TempDir()
	zp := filepath.Join(dir, "a.zip")
	f, _ := os.Create(zp)
	zw := zip.NewWriter(f)
	w, _ := zw.Create("build/bin/" + exeName("llama-server"))
	w.Write([]byte("bin"))
	w, _ = zw.Create("build/bin/libllama.so")
	w.Write([]byte("lib"))
	zw.Close()
	f.Close()
	dst := filepath.Join(dir, "out")
	if err := Extract(zp, dst); err != nil {
		t.Fatal(err)
	}
	if p := findFile(dst, exeName("llama-server")); !strings.HasSuffix(p, filepath.Join("build", "bin", exeName("llama-server"))) {
		t.Fatalf("found %q", p)
	}
	// Pfade nach außen werden abgelehnt
	f, _ = os.Create(zp)
	zw = zip.NewWriter(f)
	w, _ = zw.Create("../evil")
	w.Write([]byte("x"))
	zw.Close()
	f.Close()
	if err := Extract(zp, filepath.Join(dir, "out2")); err == nil {
		t.Fatal("zip slip not rejected")
	}
}

func TestInstalled(t *testing.T) {
	e := New(t.TempDir())
	if e.Installed() {
		t.Fatal("empty dir counts as installed")
	}
	if (*Engine)(nil).Installed() {
		t.Fatal("nil engine")
	}
}
