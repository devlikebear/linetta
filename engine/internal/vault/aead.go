package vault

import (
	"crypto/aes"
	"crypto/cipher"
)

// newAEAD makes the cipher every blob is sealed with: AES-256-GCM with a random
// 96-bit nonce written in front of the ciphertext and the 128-bit tag after it.
// That layout is the format, whichever implementation produces it. The iOS
// build seals with Apple's CryptoKit instead (aead_cryptokit.go), so the
// encryption it uses is the operating system's own; the bytes are the same.
var newAEAD = goAEAD

// sealOverhead is what sealing adds: the nonce in front and the tag behind.
const sealOverhead = 12 + 16

func goAEAD(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCMWithRandomNonce(block)
}
