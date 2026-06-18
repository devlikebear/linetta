//go:build darwin

package settings

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <CoreFoundation/CoreFoundation.h>
#include <Security/Security.h>
#include <stdlib.h>
#include <string.h>

// linetta_kc_query builds a base generic-password query dict for (service, account).
static CFMutableDictionaryRef linetta_kc_query(const char *service, const char *account) {
    CFMutableDictionaryRef q = CFDictionaryCreateMutable(
        kCFAllocatorDefault, 0,
        &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
    CFDictionarySetValue(q, kSecClass, kSecClassGenericPassword);
    CFStringRef s = CFStringCreateWithCString(kCFAllocatorDefault, service, kCFStringEncodingUTF8);
    CFStringRef a = CFStringCreateWithCString(kCFAllocatorDefault, account, kCFStringEncodingUTF8);
    CFDictionarySetValue(q, kSecAttrService, s);
    CFDictionarySetValue(q, kSecAttrAccount, a);
    CFRelease(s);
    CFRelease(a);
    return q;
}

// linetta_kc_get copies the password data. On success mallocs *out (len *outLen); caller frees.
static OSStatus linetta_kc_get(const char *service, const char *account, void **out, int *outLen) {
    CFMutableDictionaryRef q = linetta_kc_query(service, account);
    CFDictionarySetValue(q, kSecReturnData, kCFBooleanTrue);
    CFDictionarySetValue(q, kSecMatchLimit, kSecMatchLimitOne);
    CFTypeRef result = NULL;
    OSStatus status = SecItemCopyMatching(q, &result);
    CFRelease(q);
    if (status != errSecSuccess) return status;
    CFDataRef data = (CFDataRef)result;
    CFIndex len = CFDataGetLength(data);
    void *buf = malloc(len);
    memcpy(buf, CFDataGetBytePtr(data), len);
    CFRelease(result);
    *out = buf;
    *outLen = (int)len;
    return errSecSuccess;
}

// linetta_kc_exists returns errSecSuccess if present, errSecItemNotFound otherwise.
static OSStatus linetta_kc_exists(const char *service, const char *account) {
    CFMutableDictionaryRef q = linetta_kc_query(service, account);
    CFDictionarySetValue(q, kSecMatchLimit, kSecMatchLimitOne);
    OSStatus status = SecItemCopyMatching(q, NULL);
    CFRelease(q);
    return status;
}

// linetta_kc_set updates an existing item or adds a new one.
static OSStatus linetta_kc_set(const char *service, const char *account, const void *value, int valueLen) {
    CFMutableDictionaryRef q = linetta_kc_query(service, account);
    CFDataRef data = CFDataCreate(kCFAllocatorDefault, (const UInt8 *)value, valueLen);
    CFMutableDictionaryRef attrs = CFDictionaryCreateMutable(
        kCFAllocatorDefault, 0,
        &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
    CFDictionarySetValue(attrs, kSecValueData, data);
    OSStatus status = SecItemUpdate(q, attrs);
    if (status == errSecItemNotFound) {
        CFDictionarySetValue(q, kSecValueData, data);
        status = SecItemAdd(q, NULL);
    }
    CFRelease(attrs);
    CFRelease(data);
    CFRelease(q);
    return status;
}

// linetta_kc_delete removes the item.
static OSStatus linetta_kc_delete(const char *service, const char *account) {
    CFMutableDictionaryRef q = linetta_kc_query(service, account);
    OSStatus status = SecItemDelete(q);
    CFRelease(q);
    return status;
}
*/
import "C"
import (
	"fmt"
	"unsafe"
)

const (
	keychainService    = "devlikebear.linetta"
	errSecItemNotFound = -25300
)

func defaultSecretStore() SecretStore {
	return keychainSecretStore{service: keychainService}
}

type keychainSecretStore struct {
	service string
}

func (k keychainSecretStore) Get(name string) (string, bool, error) {
	cService, cAccount := C.CString(k.service), C.CString(name)
	defer C.free(unsafe.Pointer(cService))
	defer C.free(unsafe.Pointer(cAccount))

	var out unsafe.Pointer
	var outLen C.int
	status := C.linetta_kc_get(cService, cAccount, &out, &outLen)
	if status == errSecItemNotFound {
		return "", false, nil
	}
	if status != 0 {
		return "", false, keychainError("get", status)
	}
	defer C.free(out)
	return string(C.GoBytes(out, outLen)), true, nil
}

func (k keychainSecretStore) Exists(name string) (bool, error) {
	cService, cAccount := C.CString(k.service), C.CString(name)
	defer C.free(unsafe.Pointer(cService))
	defer C.free(unsafe.Pointer(cAccount))

	status := C.linetta_kc_exists(cService, cAccount)
	if status == errSecItemNotFound {
		return false, nil
	}
	if status != 0 {
		return false, keychainError("exists", status)
	}
	return true, nil
}

func (k keychainSecretStore) Set(name, value string) error {
	if value == "" {
		return k.Delete(name)
	}
	cService, cAccount := C.CString(k.service), C.CString(name)
	defer C.free(unsafe.Pointer(cService))
	defer C.free(unsafe.Pointer(cAccount))

	cValue := C.CBytes([]byte(value))
	defer C.free(cValue)

	status := C.linetta_kc_set(cService, cAccount, cValue, C.int(len(value)))
	if status != 0 {
		return keychainError("set", status)
	}
	return nil
}

func (k keychainSecretStore) Delete(name string) error {
	cService, cAccount := C.CString(k.service), C.CString(name)
	defer C.free(unsafe.Pointer(cService))
	defer C.free(unsafe.Pointer(cAccount))

	status := C.linetta_kc_delete(cService, cAccount)
	if status != 0 && status != errSecItemNotFound {
		return keychainError("delete", status)
	}
	return nil
}

func keychainError(op string, status C.OSStatus) error {
	return fmt.Errorf("settings: keychain %s failed: %d", op, int(status))
}
