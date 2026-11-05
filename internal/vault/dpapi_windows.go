//go:build windows

package vault

// DPAPI (Data Protection API) über golang.org/x/sys/windows, ohne cgo, daher
// auch für windows/arm64 und windows/386 cross-kompilierbar.
//
// CryptProtectData verschlüsselt mit einem Schlüssel, den Windows aus dem
// Anmeldegeheimnis des aktuellen Benutzers ableitet (Master Key unter
// %APPDATA%\Microsoft\Protect\<SID>). Ohne CRYPTPROTECT_LOCAL_MACHINE ist der
// Blob nur für genau dieses Konto lesbar. CRYPTPROTECT_UI_FORBIDDEN verhindert
// jede Rückfrage-UI (wir sind eine Konsolenanwendung).

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// DPAPIAvailable: unter Windows immer.
func DPAPIAvailable() bool { return true }

func blobOf(b []byte) *windows.DataBlob {
	if len(b) == 0 {
		return &windows.DataBlob{}
	}
	return &windows.DataBlob{Size: uint32(len(b)), Data: &b[0]}
}

func dpapiProtect(plain, entropy []byte) ([]byte, error) {
	var out windows.DataBlob
	desc, _ := windows.UTF16PtrFromString("sshtermy vault key")
	err := windows.CryptProtectData(blobOf(plain), desc, blobOf(entropy), 0, nil,
		windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	if err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}

func dpapiUnprotect(blob, entropy []byte) ([]byte, error) {
	var out windows.DataBlob
	err := windows.CryptUnprotectData(blobOf(blob), nil, blobOf(entropy), 0, nil,
		windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	if err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	res := append([]byte(nil), unsafe.Slice(out.Data, out.Size)...)
	// Klartext im von Windows allokierten Puffer vor der Freigabe nullen.
	wipe(unsafe.Slice(out.Data, out.Size))
	return res, nil
}
