// Package vault keeps a writer's works on Linetta Works without the site being
// able to read them. Everything leaves the device sealed under a key only the
// writer holds; the server stores opaque blobs, their sizes and their times.
//
// The sealed file format is permanent the day it ships: a blob written by this
// version has to open in every later one, or a writer's recovery key stops
// recovering anything. TestFixedVectors pins it against values computed outside
// this package. A new layout takes a new FormatVersion, never an edit to this
// one.
package vault

import (
	"bytes"
	"compress/flate"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

// A sealed blob, all of it stored on the server:
//
//	magic      4  "LNVB"
//	version    1  FormatVersion
//	flags      1  flagDeflate
//	key check  8  tells a wrong key from damage, without decrypting
//	nonce     12  random, one per seal
//	body       n  AES-256-GCM over the plaintext, deflated when that is smaller
//	tag       16  authenticates the body, everything above it, and the blob ID
const (
	magic = "LNVB"

	// FormatVersion is the sealed file format this build writes.
	FormatVersion = 1

	flagDeflate = 1 << 0

	checkSize  = 8
	headerSize = len(magic) + 1 + 1 + checkSize
	gcmExtra   = 12 + 16

	// Overhead is what sealing adds to a body.
	Overhead = headerSize + gcmExtra

	// MaxBlobBytes is the largest blob the site accepts (limits.blob_bytes in
	// GET /api/artworks/vault/status).
	MaxBlobBytes = 4 << 20

	// MaxPlaintext is the most one blob can hold when nothing compresses.
	// Callers split anything larger.
	MaxPlaintext = MaxBlobBytes - Overhead
)

// These are separate from each other because a screen says something different
// for each: re-enter your key, the copy on the server is damaged, update the
// app.
var (
	ErrBadRecoveryKey = errors.New("vault: not a valid recovery key")
	ErrWrongKey       = errors.New("vault: sealed with a different recovery key")
	ErrCorrupt        = errors.New("vault: sealed blob is damaged or was altered")
	ErrNewerFormat    = errors.New("vault: sealed by a newer version of Linetta")
	ErrTooLarge       = errors.New("vault: plaintext is larger than one blob holds")
)

const (
	recoveryKeySize      = 32
	recoveryChecksumSize = 3
	recoveryTextSize     = 56
	recoveryGroupSize    = 4
	recoveryChecksumTag  = "linetta-vault-recovery-key"
)

// recoveryEncoding is Crockford's base32: no I, L, O or U, so a key copied by
// hand has no pair of look-alike characters. 35 bytes is exactly 56 characters.
var recoveryEncoding = base32.NewEncoding("0123456789ABCDEFGHJKMNPQRSTVWXYZ").WithPadding(base32.NoPadding)

var recoveryReplacer = strings.NewReplacer("-", "", " ", "", "\t", "", "\n", "", "\r", "", "O", "0", "I", "1", "L", "1")

const redacted = "[redacted]"

// RecoveryKey is the one secret behind a writer's vault. Text is the only way
// to read it back; every fmt verb prints a placeholder and JSON sees nothing,
// so it cannot reach a log, a diagnostic or an RPC response by being passed
// along. raw is a pointer because fmt cannot call Format on an unexported field
// of a larger struct: there it prints what it finds, and for a pointer that is
// an address.
type RecoveryKey struct {
	raw *[recoveryKeySize]byte
}

// NewRecoveryKey makes a fresh key. crypto/rand does not fail.
func NewRecoveryKey() RecoveryKey {
	k := RecoveryKey{raw: new([recoveryKeySize]byte)}
	_, _ = rand.Read(k.raw[:])
	return k
}

// ParseRecoveryKey reads a key as a writer typed or pasted it. It forgives case,
// spacing and the look-alike characters, and the checksum catches what is left:
// a mistyped key fails here, not as an unexplained failure to open a blob. The
// error never carries the input.
func ParseRecoveryKey(text string) (RecoveryKey, error) {
	// The length is the text's, not the decoded bytes': a 57th character decodes
	// to the same 35 bytes, its spare bits silently dropped.
	normal := recoveryReplacer.Replace(strings.ToUpper(text))
	decoded, err := recoveryEncoding.DecodeString(normal)
	if err != nil || len(normal) != recoveryTextSize {
		return RecoveryKey{}, ErrBadRecoveryKey
	}
	k := RecoveryKey{raw: new([recoveryKeySize]byte)}
	copy(k.raw[:], decoded)
	if !bytes.Equal(decoded[recoveryKeySize:], k.checksum()) {
		return RecoveryKey{}, ErrBadRecoveryKey
	}
	return k, nil
}

func (k RecoveryKey) checksum() []byte {
	sum := sha256.Sum256(append([]byte(recoveryChecksumTag), k.raw[:]...))
	return sum[:recoveryChecksumSize]
}

// Text is the key as the writer sees and stores it: grouped, checksummed. It is
// the single exit for key material; a caller of Text owns where it goes.
func (k RecoveryKey) Text() string {
	encoded := recoveryEncoding.EncodeToString(append(k.raw[:], k.checksum()...))
	var b strings.Builder
	for i := 0; i < len(encoded); i += recoveryGroupSize {
		if i > 0 {
			b.WriteByte('-')
		}
		b.WriteString(encoded[i : i+recoveryGroupSize])
	}
	return b.String()
}

// Format keeps the key out of every fmt verb, %d and %x included.
func (RecoveryKey) Format(f fmt.State, _ rune) { _, _ = io.WriteString(f, redacted) }

// Keys are the working keys derived from a RecoveryKey.
type Keys struct {
	aead  cipher.AEAD
	idKey []byte
	check []byte
}

// Format keeps the working keys out of every fmt verb too.
func (Keys) Format(f fmt.State, _ rune) { _, _ = io.WriteString(f, redacted) }

// Derive splits the recovery key by purpose with HKDF, so the blob IDs the
// server sees and the key check in each header say nothing about the encryption
// key.
func (k RecoveryKey) Derive() (*Keys, error) {
	derive := func(purpose string, size int) ([]byte, error) {
		return hkdf.Key(sha256.New, k.raw[:], nil, "linetta-vault-v1-"+purpose, size)
	}
	encKey, err := derive("encrypt", recoveryKeySize)
	if err != nil {
		return nil, err
	}
	idKey, err := derive("blob-id", recoveryKeySize)
	if err != nil {
		return nil, err
	}
	check, err := derive("key-check", checkSize)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(encKey)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return nil, err
	}
	return &Keys{aead: aead, idKey: idKey, check: check}, nil
}

// BlobID names a plaintext for this writer: 64 lowercase hex characters, the
// form the site requires. It is keyed, so the server cannot test a guess at the
// content against it, and it is taken over the plaintext, so the same part is
// not uploaded twice however it was compressed.
func (k *Keys) BlobID(plain []byte) string {
	mac := hmac.New(sha256.New, k.idKey)
	mac.Write(plain)
	return hex.EncodeToString(mac.Sum(nil))
}

// Seal encrypts one blob. Two seals of the same plaintext differ byte for byte
// and share an ID; either opens, and the site keeps whichever arrived first.
func (k *Keys) Seal(plain []byte) (id string, sealed []byte, err error) {
	if len(plain) > MaxPlaintext {
		return "", nil, ErrTooLarge
	}
	body, flags, err := pack(plain)
	if err != nil {
		return "", nil, err
	}
	id = k.BlobID(plain)
	return id, k.seal(id, flags, body), nil
}

func (k *Keys) seal(id string, flags byte, body []byte) []byte {
	header := make([]byte, 0, headerSize+gcmExtra+len(body))
	header = append(header, magic...)
	header = append(header, FormatVersion, flags)
	header = append(header, k.check...)
	return k.aead.Seal(header, nil, body, additionalData(header, id))
}

// additionalData binds the header and the blob's name to its body. Without the
// name, the server could answer a request for one blob with another of the
// writer's own, and it would still decrypt.
func additionalData(header []byte, id string) []byte {
	return append(bytes.Clone(header[:headerSize]), id...)
}

// Open reverses Seal and proves the result is what id names. The checks run from
// cheapest and most specific to most general, so each failure is reported as
// what it is: a blob from a newer app or under another key never reaches
// decryption, where both would only look like damage.
func (k *Keys) Open(id string, sealed []byte) ([]byte, error) {
	if len(sealed) < Overhead || string(sealed[:len(magic)]) != magic {
		return nil, ErrCorrupt
	}
	if sealed[len(magic)] > FormatVersion {
		return nil, ErrNewerFormat
	}
	if !hmac.Equal(sealed[len(magic)+2:headerSize], k.check) {
		return nil, ErrWrongKey
	}
	body, err := k.aead.Open(nil, nil, sealed[headerSize:], additionalData(sealed, id))
	if err != nil {
		return nil, ErrCorrupt
	}
	plain, err := unpack(body, sealed[len(magic)+1])
	if err != nil {
		return nil, err
	}
	if !hmac.Equal([]byte(k.BlobID(plain)), []byte(id)) {
		return nil, ErrCorrupt
	}
	return plain, nil
}

// pack compresses inside the sealed format rather than before Seal, so a blob's
// ID stays the plaintext's and not the output of whichever deflate this build
// has. Pictures do not compress, and those go in as they are.
func pack(plain []byte) ([]byte, byte, error) {
	var out bytes.Buffer
	w, err := flate.NewWriter(&out, flate.DefaultCompression)
	if err != nil {
		return nil, 0, err
	}
	if _, err := w.Write(plain); err != nil {
		return nil, 0, err
	}
	if err := w.Close(); err != nil {
		return nil, 0, err
	}
	if out.Len() < len(plain) {
		return out.Bytes(), flagDeflate, nil
	}
	return plain, 0, nil
}

// unpack never inflates past what Seal could have accepted.
func unpack(body []byte, flags byte) ([]byte, error) {
	if flags&flagDeflate == 0 {
		return body, nil
	}
	r := flate.NewReader(bytes.NewReader(body))
	defer r.Close()
	plain, err := io.ReadAll(io.LimitReader(r, int64(MaxPlaintext)+1))
	if err != nil || len(plain) > MaxPlaintext {
		return nil, ErrCorrupt
	}
	return plain, nil
}
