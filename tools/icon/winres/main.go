// Command winres schreibt das App-Icon als Windows-Ressource (.syso) neben
// main.go, damit Explorer, Taskleiste und Startmenü sshtermy.exe mit Icon
// zeigen. go build nimmt rsrc_windows_<arch>.syso automatisch mit; andere
// Systeme ignorieren die Dateien.
//
//	go run ./tools/icon/winres            (aus assets/icon/sshtermy.ico)
//
// Ohne Fremdwerkzeug: die .ico-Bilder werden als RT_ICON + RT_GROUP_ICON in
// eine COFF-Objektdatei mit einer .rsrc-Sektion gepackt.
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

const (
	rtIcon      = 3
	rtGroupIcon = 14
	langNeutral = 0
	groupID     = 1
)

type icoImage struct {
	w, h, colors, planes, bits uint16
	data                       []byte
}

func readICO(path string) ([]icoImage, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	le := binary.LittleEndian
	if len(b) < 6 || le.Uint16(b[0:]) != 0 || le.Uint16(b[2:]) != 1 {
		return nil, fmt.Errorf("%s: keine .ico-Datei", path)
	}
	n := int(le.Uint16(b[4:]))
	var out []icoImage
	for i := 0; i < n; i++ {
		e := b[6+16*i:]
		if len(e) < 16 {
			return nil, fmt.Errorf("%s: Eintrag %d abgeschnitten", path, i)
		}
		size, off := le.Uint32(e[8:]), le.Uint32(e[12:])
		if int(off)+int(size) > len(b) {
			return nil, fmt.Errorf("%s: Bild %d außerhalb der Datei", path, i)
		}
		out = append(out, icoImage{
			w: uint16(e[0]), h: uint16(e[1]), colors: uint16(e[2]),
			planes: le.Uint16(e[4:]), bits: le.Uint16(e[6:]),
			data: b[off : off+size],
		})
	}
	return out, nil
}

// leaf ist ein Ressourcen-Eintrag Typ/ID/Sprache → Daten.
type leaf struct {
	typ, id uint16
	data    []byte
}

// buildRsrc baut den Inhalt der .rsrc-Sektion und liefert die Offsets der
// OffsetToData-Felder, die der Linker auf RVAs umrechnen muss.
func buildRsrc(leaves []leaf) ([]byte, []uint32) {
	// Gruppen nach Typ (aufsteigend, wie Windows es verlangt)
	var types []uint16
	byType := map[uint16][]leaf{}
	for _, l := range leaves {
		if _, ok := byType[l.typ]; !ok {
			types = append(types, l.typ)
		}
		byType[l.typ] = append(byType[l.typ], l)
	}
	for i := range types { // kleine Listen: einfaches Sortieren
		for j := i + 1; j < len(types); j++ {
			if types[j] < types[i] {
				types[i], types[j] = types[j], types[i]
			}
		}
	}

	dirSize := func(n int) int { return 16 + 8*n }
	// Layout: Wurzel, Typ-Verzeichnisse, Sprach-Verzeichnisse, Daten-Einträge, Daten
	off := dirSize(len(types))
	typeOff := map[uint16]int{}
	for _, t := range types {
		typeOff[t] = off
		off += dirSize(len(byType[t]))
	}
	langOff := make([]int, len(leaves))
	ordered := make([]leaf, 0, len(leaves))
	for _, t := range types {
		ordered = append(ordered, byType[t]...)
	}
	for i := range ordered {
		langOff[i] = off
		off += dirSize(1)
	}
	entryOff := make([]int, len(ordered))
	for i := range ordered {
		entryOff[i] = off
		off += 16
	}
	dataOff := make([]int, len(ordered))
	for i, l := range ordered {
		off = (off + 7) &^ 7
		dataOff[i] = off
		off += len(l.data)
	}
	buf := make([]byte, (off+7)&^7)
	le := binary.LittleEndian
	dir := func(at, n int) {
		le.PutUint16(buf[at+14:], uint16(n)) // nur ID-Einträge
	}
	ent := func(at int, id uint16, target int, sub bool) {
		le.PutUint32(buf[at:], uint32(id))
		v := uint32(target)
		if sub {
			v |= 0x80000000
		}
		le.PutUint32(buf[at+4:], v)
	}

	dir(0, len(types))
	for i, t := range types {
		ent(16+8*i, t, typeOff[t], true)
	}
	k := 0
	for _, t := range types {
		dir(typeOff[t], len(byType[t]))
		for j, l := range byType[t] {
			ent(typeOff[t]+16+8*j, l.id, langOff[k], true)
			k++
		}
	}
	var relocs []uint32
	for i, l := range ordered {
		dir(langOff[i], 1)
		ent(langOff[i]+16, langNeutral, entryOff[i], false)
		le.PutUint32(buf[entryOff[i]:], uint32(dataOff[i])) // + RVA der Sektion (Relokation)
		le.PutUint32(buf[entryOff[i]+4:], uint32(len(l.data)))
		relocs = append(relocs, uint32(entryOff[i]))
		copy(buf[dataOff[i]:], l.data)
	}
	return buf, relocs
}

// coff verpackt die Sektion als Objektdatei für den Go-Linker.
func coff(machine, relType uint16, sect []byte, relocs []uint32) []byte {
	var b bytes.Buffer
	le := binary.LittleEndian
	w := func(v any) { binary.Write(&b, le, v) }
	const hdr, shdr = 20, 40
	rawOff := hdr + shdr
	relOff := rawOff + len(sect)
	symOff := relOff + 10*len(relocs)

	w(machine)
	w(uint16(1)) // Sektionen
	w(uint32(0)) // Zeitstempel: reproduzierbar
	w(uint32(symOff))
	w(uint32(1)) // Symbole
	w(uint16(0)) // optionaler Header
	w(uint16(0))

	b.WriteString(".rsrc\x00\x00\x00")
	w(uint32(0)) // VirtualSize
	w(uint32(0)) // VirtualAddress
	w(uint32(len(sect)))
	w(uint32(rawOff))
	w(uint32(relOff))
	w(uint32(0))
	w(uint16(len(relocs)))
	w(uint16(0))
	w(uint32(0x40000040)) // INITIALIZED_DATA | MEM_READ

	b.Write(sect)
	for _, r := range relocs {
		w(r)
		w(uint32(0)) // Symbol .rsrc
		w(relType)
	}
	b.WriteString(".rsrc\x00\x00\x00")
	w(uint32(0)) // Wert
	w(uint16(1)) // Sektion 1
	w(uint16(0)) // Typ
	w(uint8(3))  // IMAGE_SYM_CLASS_STATIC
	w(uint8(0))  // keine Hilfssymbole
	w(uint32(4)) // leere String-Tabelle
	return b.Bytes()
}

func main() {
	ico := flag.String("ico", "assets/icon/sshtermy.ico", "Icon-Datei")
	out := flag.String("out", "cmd/sshtermy", "Zielordner der .syso-Dateien")
	flag.Parse()

	imgs, err := readICO(*ico)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var leaves []leaf
	grp := new(bytes.Buffer)
	le := binary.LittleEndian
	binary.Write(grp, le, [3]uint16{0, 1, uint16(len(imgs))})
	for i, im := range imgs {
		id := uint16(i + 1)
		leaves = append(leaves, leaf{rtIcon, id, im.data})
		grp.Write([]byte{byte(im.w), byte(im.h), byte(im.colors), 0})
		binary.Write(grp, le, im.planes)
		binary.Write(grp, le, im.bits)
		binary.Write(grp, le, uint32(len(im.data)))
		binary.Write(grp, le, id)
	}
	leaves = append(leaves, leaf{rtGroupIcon, groupID, grp.Bytes()})
	sect, relocs := buildRsrc(leaves)

	for _, a := range []struct {
		arch             string
		machine, relType uint16
	}{
		{"amd64", 0x8664, 3}, // IMAGE_REL_AMD64_ADDR32NB
		{"386", 0x14c, 7},    // IMAGE_REL_I386_DIR32NB
		{"arm64", 0xaa64, 2}, // IMAGE_REL_ARM64_ADDR32NB
	} {
		p := filepath.Join(*out, "rsrc_windows_"+a.arch+".syso")
		if err := os.WriteFile(p, coff(a.machine, a.relType, sect, relocs), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println(p)
	}
}
