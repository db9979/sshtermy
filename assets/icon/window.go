package icon

import (
	"bytes"
	"embed"
	"image"
	"image/png"
)

//go:embed sshtermy-16.png sshtermy-32.png sshtermy-48.png sshtermy-256.png
var windowPNGs embed.FS

// Window liefert das Fenstersymbol in mehreren Größen (Titelleiste,
// Taskleiste, Alt+Tab); das System nimmt die passende.
func Window() []image.Image {
	var out []image.Image
	for _, n := range []string{"sshtermy-16.png", "sshtermy-32.png", "sshtermy-48.png", "sshtermy-256.png"} {
		b, err := windowPNGs.ReadFile(n)
		if err != nil {
			continue
		}
		if im, err := png.Decode(bytes.NewReader(b)); err == nil {
			out = append(out, im)
		}
	}
	return out
}
