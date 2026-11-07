//go:build cgo && (ios || (darwin && cryptokit))

package vault

/*
long linetta_cryptokit_seal(const unsigned char *key, long key_len,
	const unsigned char *plain, long plain_len,
	const unsigned char *aad, long aad_len,
	unsigned char *out, long out_cap);
long linetta_cryptokit_open(const unsigned char *key, long key_len,
	const unsigned char *sealed, long sealed_len,
	const unsigned char *aad, long aad_len,
	unsigned char *out, long out_cap);
*/
import "C"

import (
	"crypto/cipher"
	"errors"
	"unsafe"
)

// On iOS the blobs are sealed by CryptoKit (engine/cryptokit), linked into the
// app beside this engine. Building the engine for iOS without it fails to link,
// which is the point: nothing on iOS seals with Go's own AES.
func init() { newAEAD = cryptoKitAEAD }

var errCryptoKitOpen = errors.New("vault: CryptoKit could not open the blob")

type cryptoKit struct{ key []byte }

func cryptoKitAEAD(key []byte) (cipher.AEAD, error) {
	if len(key) != 32 {
		return nil, errors.New("vault: CryptoKit needs a 256-bit key")
	}
	return cryptoKit{key: append([]byte(nil), key...)}, nil
}

func bytesPtr(b []byte) *C.uchar {
	if len(b) == 0 {
		return nil
	}
	return (*C.uchar)(unsafe.Pointer(&b[0]))
}

// NonceSize is zero: like cipher.NewGCMWithRandomNonce, the nonce is made
// inside and travels in front of the ciphertext.
func (cryptoKit) NonceSize() int { return 0 }

func (cryptoKit) Overhead() int { return sealOverhead }

func (c cryptoKit) Seal(dst, nonce, plaintext, additionalData []byte) []byte {
	if len(nonce) != 0 {
		panic("vault: CryptoKit makes its own nonce")
	}
	out := make([]byte, len(dst)+len(plaintext)+sealOverhead)
	copy(out, dst)
	body := out[len(dst):]
	n := C.linetta_cryptokit_seal(bytesPtr(c.key), C.long(len(c.key)),
		bytesPtr(plaintext), C.long(len(plaintext)),
		bytesPtr(additionalData), C.long(len(additionalData)),
		bytesPtr(body), C.long(len(body)))
	if int(n) != len(body) {
		// Only a key of the wrong size makes CryptoKit refuse to seal, and
		// cryptoKitAEAD has already turned that away.
		panic("vault: CryptoKit could not seal")
	}
	return out
}

func (c cryptoKit) Open(dst, nonce, ciphertext, additionalData []byte) ([]byte, error) {
	if len(nonce) != 0 {
		panic("vault: CryptoKit reads its nonce from the blob")
	}
	if len(ciphertext) < sealOverhead {
		return nil, errCryptoKitOpen
	}
	plain := make([]byte, len(ciphertext)-sealOverhead)
	n := C.linetta_cryptokit_open(bytesPtr(c.key), C.long(len(c.key)),
		bytesPtr(ciphertext), C.long(len(ciphertext)),
		bytesPtr(additionalData), C.long(len(additionalData)),
		bytesPtr(plain), C.long(len(plain)))
	if int(n) != len(plain) {
		return nil, errCryptoKitOpen
	}
	return append(dst, plain...), nil
}
