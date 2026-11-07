package keychain

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows: Anmeldeinformationsverwaltung („Generische Anmeldeinformationen“),
// gebunden an das Windows-Konto. Ziel heißt „sshTermy:<Konto>“.

var (
	advapi       = windows.NewLazySystemDLL("advapi32.dll")
	procWrite    = advapi.NewProc("CredWriteW")
	procRead     = advapi.NewProc("CredReadW")
	procDelete   = advapi.NewProc("CredDeleteW")
	procCredFree = advapi.NewProc("CredFree")
)

const (
	credTypeGeneric         = 1
	credPersistLocalMachine = 2
	errNotFound             = syscall.Errno(1168) // ERROR_NOT_FOUND
)

type credential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        windows.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

type system struct{}

func (system) Available() bool { return procWrite.Find() == nil }

func target(account string) (*uint16, error) {
	return windows.UTF16PtrFromString(Service + ":" + account)
}

func (system) Set(account, label, secret string) error {
	t, err := target(account)
	if err != nil {
		return err
	}
	c, _ := windows.UTF16PtrFromString(label)
	u, _ := windows.UTF16PtrFromString(account)
	blob := []byte(secret)
	if len(blob) > 2560 {
		return errors.New("keychain: secret too long")
	}
	cred := credential{Type: credTypeGeneric, TargetName: t, Comment: c, UserName: u, Persist: credPersistLocalMachine,
		CredentialBlobSize: uint32(len(blob))}
	if len(blob) > 0 {
		cred.CredentialBlob = &blob[0]
	}
	r, _, e := procWrite.Call(uintptr(unsafe.Pointer(&cred)), 0)
	for i := range blob {
		blob[i] = 0
	}
	if r == 0 {
		return fmt.Errorf("keychain: CredWrite: %w", e)
	}
	return nil
}

func (system) Get(account string) (string, error) {
	t, err := target(account)
	if err != nil {
		return "", err
	}
	var p *credential
	r, _, e := procRead.Call(uintptr(unsafe.Pointer(t)), credTypeGeneric, 0, uintptr(unsafe.Pointer(&p)))
	if r == 0 {
		if errors.Is(e, errNotFound) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("keychain: CredRead: %w", e)
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(p)))
	if p.CredentialBlobSize == 0 || p.CredentialBlob == nil {
		return "", nil
	}
	return string(unsafe.Slice(p.CredentialBlob, p.CredentialBlobSize)), nil
}

func (system) Delete(account string) error {
	t, err := target(account)
	if err != nil {
		return err
	}
	r, _, e := procDelete.Call(uintptr(unsafe.Pointer(t)), credTypeGeneric, 0)
	if r == 0 {
		if errors.Is(e, errNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("keychain: CredDelete: %w", e)
	}
	return nil
}
