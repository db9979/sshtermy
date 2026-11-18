package packx

import (
	"archive/tar"
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func tree(t *testing.T) (string, []string) {
	t.Helper()
	d := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(d, "web", "conf.d"), 0o755))
	must(os.WriteFile(filepath.Join(d, "web", "nginx.conf"), []byte("worker_processes auto;\n"), 0o644))
	must(os.WriteFile(filepath.Join(d, "web", "conf.d", "site.conf"), []byte("server {}\n"), 0o644))
	must(os.WriteFile(filepath.Join(d, "readme.txt"), []byte("hallo"), 0o644))
	return d, []string{filepath.Join(d, "web"), filepath.Join(d, "readme.txt")}
}

func TestCreateExtractRoundTrip(t *testing.T) {
	for _, ext := range []string{".zip", ".tar.gz", ".tar"} {
		t.Run(ext, func(t *testing.T) {
			d, srcs := tree(t)
			arc := filepath.Join(d, "pack"+ext)
			if err := Create(context.Background(), arc, srcs); err != nil {
				t.Fatal(err)
			}
			if err := Create(context.Background(), arc, srcs); err == nil {
				t.Fatal("vorhandenes Archiv darf nicht überschrieben werden")
			}
			out := filepath.Join(d, "aus")
			if err := Extract(context.Background(), arc, out); err != nil {
				t.Fatal(err)
			}
			b, err := os.ReadFile(filepath.Join(out, "web", "conf.d", "site.conf"))
			if err != nil || string(b) != "server {}\n" {
				t.Fatalf("site.conf: %q %v", b, err)
			}
			if b, _ := os.ReadFile(filepath.Join(out, "readme.txt")); string(b) != "hallo" {
				t.Fatalf("readme: %q", b)
			}
			// Inhalt listen
			f, _ := os.Open(arc)
			defer f.Close()
			st, _ := f.Stat()
			es, more, err := List(arc, f, f, st.Size(), 100)
			if err != nil || more {
				t.Fatal(err, more)
			}
			var names []string
			for _, e := range es {
				names = append(names, e.Name)
			}
			sort.Strings(names)
			got := strings.Join(names, ",")
			if !strings.Contains(got, "web/conf.d/site.conf") || !strings.Contains(got, "readme.txt") {
				t.Fatalf("Liste: %s", got)
			}
			es, more, _ = List(arc, mustOpen(t, arc), mustOpen(t, arc), st.Size(), 2)
			if len(es) != 2 || !more {
				t.Fatalf("Begrenzung: %d %v", len(es), more)
			}
		})
	}
}

func mustOpen(t *testing.T, p string) *os.File {
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

// Archive mit „../“ oder absoluten Namen schreiben nie außerhalb.
func TestExtractRejectsEscape(t *testing.T) {
	d := t.TempDir()
	arc := filepath.Join(d, "bad.zip")
	f, _ := os.Create(arc)
	zw := zip.NewWriter(f)
	w, _ := zw.Create("../../boese.txt")
	w.Write([]byte("x"))
	zw.Close()
	f.Close()
	if err := Extract(context.Background(), arc, filepath.Join(d, "aus")); err == nil {
		t.Fatal("../ muss abbrechen")
	}
	if _, err := os.Stat(filepath.Join(d, "..", "boese.txt")); err == nil {
		t.Fatal("Datei außerhalb geschrieben")
	}

	arc = filepath.Join(d, "link.tar")
	f, _ = os.Create(arc)
	tw := tar.NewWriter(f)
	tw.WriteHeader(&tar.Header{Name: "raus", Typeflag: tar.TypeSymlink, Linkname: "../../etc"})
	tw.WriteHeader(&tar.Header{Name: "drin", Typeflag: tar.TypeSymlink, Linkname: "sub/x"})
	tw.Close()
	f.Close()
	out := filepath.Join(d, "aus2")
	if err := Extract(context.Background(), arc, out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(out, "raus")); err == nil {
		t.Fatal("Link nach draußen angelegt")
	}
}

func TestFormatAndStem(t *testing.T) {
	cases := map[string]Format{"a.zip": Zip, "a.TAR.GZ": TarGz, "a.tgz": TarGz, "a.tar": Tar, "a.tar.xz": TarXz, "a.txt": None, "a.tar.zst": TarZst}
	for n, f := range cases {
		if FormatOf(n) != f {
			t.Errorf("%s: %v", n, FormatOf(n))
		}
	}
	if Stem("logs.tar.gz") != "logs" || Stem("x.zip") != "x" || Stem("plain") != "plain" || Stem(".zip") != ".zip" {
		t.Fatal("Stem")
	}
}

func TestCommands(t *testing.T) {
	c, err := PackCommand("/var/log", "alt's.tar.gz", []string{"nginx", "-x"})
	if err != nil || c != `cd '/var/log' && tar -czf 'alt'\''s.tar.gz' './nginx' './-x'` {
		t.Fatalf("%q %v", c, err)
	}
	if _, err := PackCommand("/", "a.7z", []string{"x"}); err == nil {
		t.Fatal("7z gibt es nicht")
	}
	c, _ = ExtractCommand("/srv/a.zip", "/srv/a")
	if !strings.HasPrefix(c, "mkdir -p '/srv/a' && if command -v unzip") || !strings.Contains(c, "python3 -m zipfile -e '/srv/a.zip' '/srv/a'") {
		t.Fatalf("%q", c)
	}
	c, _ = ExtractCommand("/srv/a.tgz", "/srv/a")
	if c != "mkdir -p '/srv/a' && tar -xzf '/srv/a.tgz' -C '/srv/a'" {
		t.Fatalf("%q", c)
	}
}
