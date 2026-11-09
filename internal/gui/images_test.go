package gui

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/db9979/sshtermy/internal/termemu"
)

// Bilddaten verschwinden aus dem Strom, auch über mehrere Writes verteilt,
// und das Bild liegt danach unter seiner Nummer bereit.
func TestImageLayerChunks(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 2))
	img.Set(0, 0, color.NRGBA{255, 0, 0, 255})
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	enc := base64.StdEncoding.EncodeToString(buf.Bytes())
	half := len(enc) / 2
	stream := fmt.Sprintf("vorher\x1b_Ga=T,U=1,f=100,q=2,i=%d,c=2,r=1,m=1;%s\x1b\\\x1b_Gm=0;%s\x1b\\nachher", 0x5e0001, enc[:half], enc[half:])

	l := newImageLayer()
	var out []byte
	// in kleinen Stücken, damit Sequenzanfang und -ende zerschnitten werden
	for i := 0; i < len(stream); i += 7 {
		out = append(out, l.filter([]byte(stream[i:min(i+7, len(stream))]), nil)...)
	}
	if string(out) != "vorhernachher" {
		t.Fatalf("Rest %q", out)
	}
	ki := l.get(0x5e0001)
	if ki == nil || ki.cols != 2 || ki.rows != 1 || ki.img.Bounds().Dx() != 4 {
		t.Fatalf("Bild fehlt oder falsch: %+v", ki)
	}
}

func TestPlaceholderCell(t *testing.T) {
	d := termemu.KittyDiacritics()
	content := string([]rune{placeholder, d[3], d[7]})
	id, row, col, ok := placeholderCell(content, 0x5e, 0x00, 0x2a)
	if !ok || id != 0x5e002a || row != 3 || col != 7 {
		t.Fatalf("id=%x row=%d col=%d ok=%v", id, row, col, ok)
	}
	if _, _, _, ok := placeholderCell("a", 0, 0, 0); ok {
		t.Fatal("normaler Text ist kein Platzhalter")
	}
}

func TestQuotePaths(t *testing.T) {
	if got := quotePaths([]string{"/a b/c.txt", `C:\x.txt`}); got != `"/a b/c.txt" "C:\x.txt"` {
		t.Fatal(got)
	}
}

// Wie Ebitens DroppedFiles: jede Datei öffnet sich als *os.File mit echtem Pfad.
func TestDroppedPaths(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o600)
	_ = os.Mkdir(filepath.Join(dir, "ordner"), 0o700)
	_ = os.WriteFile(filepath.Join(dir, "ordner", "b.txt"), []byte("y"), 0o600)
	got := droppedPaths(os.DirFS(dir))
	want := []string{filepath.Join(dir, "a.txt"), filepath.Join(dir, "ordner")}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("%v, erwartet %v", got, want)
	}
}
