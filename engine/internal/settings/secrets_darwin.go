//go:build darwin

package settings

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <CoreFoundation/CoreFoundation.h>
#include <Security/Security.h>
#include <stdlib.h>
*/
import "C"
import (
	"fmt"
	"unsafe"
)

const (
	keychainService     = "devlikebear.linetta"
	errSecNotFound      = -25300
	errSecDuplicateItem = -25299
)

func defaultSecretStore() SecretStore {
	return keychainSecretStore{service: keychainService}
}

type keychainSecretStore struct {
	service string
}

func (k keychainSecretStore) Get(name string) (string, bool, error) {
	service, account := cString(k.service), cString(name)
	defer service.free()
	defer account.free()

	var passwordLen C.UInt32
	var passwordData unsafe.Pointer
	status := C.SecKeychainFindGenericPassword(
		C.CFTypeRef(0),
		service.len,
		service.ptr,
		account.len,
		account.ptr,
		&passwordLen,
		&passwordData,
		nil,
	)
	if status == errSecNotFound {
		return "", false, nil
	}
	if status != 0 {
		return "", false, keychainError("find", status)
	}
	defer C.SecKeychainItemFreeContent(nil, passwordData)
	return string(C.GoBytes(passwordData, C.int(passwordLen))), true, nil
}

func (k keychainSecretStore) Exists(name string) (bool, error) {
	item, ok, err := k.findItem(name)
	if err != nil || !ok {
		return ok, err
	}
	defer C.CFRelease(C.CFTypeRef(item))
	return true, nil
}

func (k keychainSecretStore) Set(name, value string) error {
	if value == "" {
		return k.Delete(name)
	}
	service, account, password := cString(k.service), cString(name), cString(value)
	defer service.free()
	defer account.free()
	defer password.free()

	status := C.SecKeychainAddGenericPassword(
		C.SecKeychainRef(0),
		service.len,
		service.ptr,
		account.len,
		account.ptr,
		password.len,
		unsafe.Pointer(password.ptr),
		nil,
	)
	if status == errSecDuplicateItem {
		return k.update(name, value)
	}
	if status != 0 {
		return keychainError("add", status)
	}
	return nil
}

func (k keychainSecretStore) Delete(name string) error {
	item, ok, err := k.findItem(name)
	if err != nil || !ok {
		return err
	}
	defer C.CFRelease(C.CFTypeRef(item))
	status := C.SecKeychainItemDelete(item)
	if status != 0 && status != errSecNotFound {
		return keychainError("delete", status)
	}
	return nil
}

func (k keychainSecretStore) update(name, value string) error {
	item, ok, err := k.findItem(name)
	if err != nil || !ok {
		return err
	}
	defer C.CFRelease(C.CFTypeRef(item))
	password := cString(value)
	defer password.free()
	status := C.SecKeychainItemModifyAttributesAndData(
		item,
		nil,
		password.len,
		unsafe.Pointer(password.ptr),
	)
	if status != 0 {
		return keychainError("update", status)
	}
	return nil
}

func (k keychainSecretStore) findItem(name string) (C.SecKeychainItemRef, bool, error) {
	service, account := cString(k.service), cString(name)
	defer service.free()
	defer account.free()
	var item C.SecKeychainItemRef
	status := C.SecKeychainFindGenericPassword(
		C.CFTypeRef(0),
		service.len,
		service.ptr,
		account.len,
		account.ptr,
		nil,
		nil,
		&item,
	)
	if status == errSecNotFound {
		return C.SecKeychainItemRef(0), false, nil
	}
	if status != 0 {
		return C.SecKeychainItemRef(0), false, keychainError("find item", status)
	}
	return item, true, nil
}

type keychainCString struct {
	ptr *C.char
	len C.UInt32
}

func cString(value string) keychainCString {
	ptr := C.CString(value)
	return keychainCString{ptr: ptr, len: C.UInt32(len(value))}
}

func (s keychainCString) free() {
	C.free(unsafe.Pointer(s.ptr))
}

func keychainError(op string, status C.OSStatus) error {
	return fmt.Errorf("settings: keychain %s failed: %d", op, int(status))
}
