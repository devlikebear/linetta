//go:build windows

package settings

import (
	"errors"
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows Credential Manager backend. golang.org/x/sys/windows does not wrap
// the Cred* family, so advapi32 is called directly.
//
// Secrets live as generic credentials named "devlikebear.linetta:<name>",
// persisted for the current user on this machine (never roaming). The user can
// inspect and remove them from Control Panel → Credential Manager → Windows
// Credentials, which is the same escape hatch Keychain Access gives on macOS.
var (
	advapi32        = windows.NewLazySystemDLL("advapi32.dll")
	procCredReadW   = advapi32.NewProc("CredReadW")
	procCredWriteW  = advapi32.NewProc("CredWriteW")
	procCredDeleteW = advapi32.NewProc("CredDeleteW")
	procCredFree    = advapi32.NewProc("CredFree")
)

const (
	credentialTargetPrefix = "devlikebear.linetta:"

	credTypeGeneric         = 1 // CRED_TYPE_GENERIC
	credPersistLocalMachine = 2 // CRED_PERSIST_LOCAL_MACHINE

	// CRED_MAX_CREDENTIAL_BLOB_SIZE. CredWriteW rejects anything larger, so
	// reject it here with a message that says what actually went wrong.
	credMaxBlobSize = 5 * 512
)

func defaultSecretStore() SecretStore {
	return credentialSecretStore{prefix: credentialTargetPrefix}
}

type credentialSecretStore struct {
	prefix string
}

// credentialW mirrors CREDENTIALW from wincred.h. The field order must match
// the Win32 layout exactly — the struct is handed to CredWriteW by pointer.
// Go inserts the same alignment padding C does on both 386 and amd64.
type credentialW struct {
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

func (c credentialSecretStore) target(name string) string {
	return c.prefix + name
}

func (c credentialSecretStore) Get(name string) (string, bool, error) {
	target, err := windows.UTF16PtrFromString(c.target(name))
	if err != nil {
		return "", false, credentialError("get", err)
	}

	var cred *credentialW
	ret, _, callErr := procCredReadW.Call(
		uintptr(unsafe.Pointer(target)),
		uintptr(credTypeGeneric),
		0,
		uintptr(unsafe.Pointer(&cred)),
	)
	if ret == 0 {
		if isCredentialNotFound(callErr) {
			return "", false, nil
		}
		return "", false, credentialError("get", callErr)
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(cred)))

	if cred.CredentialBlobSize == 0 || cred.CredentialBlob == nil {
		return "", true, nil
	}
	// Copy out before CredFree releases the buffer.
	blob := unsafe.Slice(cred.CredentialBlob, cred.CredentialBlobSize)
	return string(append([]byte(nil), blob...)), true, nil
}

func (c credentialSecretStore) Exists(name string) (bool, error) {
	_, ok, err := c.Get(name)
	return ok, err
}

func (c credentialSecretStore) Set(name, value string) error {
	if value == "" {
		return c.Delete(name)
	}
	blob := []byte(value)
	if len(blob) > credMaxBlobSize {
		return fmt.Errorf("settings: credential value is %d bytes, over the %d byte Windows limit", len(blob), credMaxBlobSize)
	}

	target, err := windows.UTF16PtrFromString(c.target(name))
	if err != nil {
		return credentialError("set", err)
	}
	// Generic credentials want a user name; the secret's own name makes the
	// entry self-describing in the Credential Manager UI.
	user, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return credentialError("set", err)
	}

	cred := credentialW{
		Type:               credTypeGeneric,
		TargetName:         target,
		CredentialBlobSize: uint32(len(blob)),
		CredentialBlob:     &blob[0],
		Persist:            credPersistLocalMachine,
		UserName:           user,
	}
	ret, _, callErr := procCredWriteW.Call(uintptr(unsafe.Pointer(&cred)), 0)
	// cred points at blob, target and user rather than passing them as call
	// arguments, so nothing else keeps them reachable for the duration.
	runtime.KeepAlive(blob)
	runtime.KeepAlive(target)
	runtime.KeepAlive(user)
	if ret == 0 {
		return credentialError("set", callErr)
	}
	return nil
}

func (c credentialSecretStore) Delete(name string) error {
	target, err := windows.UTF16PtrFromString(c.target(name))
	if err != nil {
		return credentialError("delete", err)
	}
	ret, _, callErr := procCredDeleteW.Call(
		uintptr(unsafe.Pointer(target)),
		uintptr(credTypeGeneric),
		0,
	)
	if ret == 0 && !isCredentialNotFound(callErr) {
		return credentialError("delete", callErr)
	}
	return nil
}

// isCredentialNotFound reports whether the Cred* call failed only because the
// entry is absent, which every caller treats as "no secret" rather than error.
func isCredentialNotFound(err error) bool {
	var errno windows.Errno
	if !errors.As(err, &errno) {
		return false
	}
	return errno == windows.ERROR_NOT_FOUND || errno == windows.ERROR_FILE_NOT_FOUND
}

func credentialError(op string, err error) error {
	return fmt.Errorf("settings: windows credential %s failed: %w", op, err)
}
