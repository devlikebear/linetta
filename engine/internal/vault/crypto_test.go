package vault

import (
	"bytes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"
)

// These values were computed by a separate script with Python's hmac and
// zlib and the cryptography package's AESGCM, from the layout documented in
// crypto.go, not by this package. The recovery key is bytes 0..31. A failure
// here means blobs already on the server no longer open: never update a vector
// to make it pass. Add a FormatVersion.
const vectorRecoveryKey = "000G-40R4-0M30-E209-185G-R38E-1W81-24GK-2GAH-C5RR-34D1-P70X-3RFW-RGSA"

var fixedVectors = []struct {
	name   string
	plain  string
	id     string
	sealed string
}{
	{
		name:   "stored",
		plain:  "홍길동이 문을 열었다.",
		id:     "34d118a1d368086ee25a7a064bc7045c3c241a7f14dc2f05ed660d6c2a548c2f",
		sealed: "4c4e56420100457e497753ef94ec000102030405060708090a0bb88aafd83239ebc09dbcfefd66c305eac17149909eba5cbed580095938442b8231656c9b6c974c3c3b2179d12f28",
	},
	{
		name:   "deflated",
		plain:  strings.Repeat("비 내리는 밤이었다.\n", 40),
		id:     "4d167f3979c2bd24de1da62d85b4e7e8bd5b0ff344fe1e7f27529477bff671d6",
		sealed: "4c4e56420101457e497753ef94ec0c0d0e0f101112131415161702b9865c7f03f09a8d251ab3b7e4f648fdfdb2bcc848cef4a537686885d93499ac2b880f58c73369c806e6943b3e0d50bb678c33522147dd9c8a0c",
	},
}

func vectorKeys(t *testing.T) *Keys {
	t.Helper()
	key, err := ParseRecoveryKey(vectorRecoveryKey)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := key.Derive()
	if err != nil {
		t.Fatal(err)
	}
	return keys
}

func newKeys(t *testing.T) *Keys {
	t.Helper()
	keys, err := NewRecoveryKey().Derive()
	if err != nil {
		t.Fatal(err)
	}
	return keys
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}

func TestFixedVectors(t *testing.T) {
	key, err := ParseRecoveryKey(vectorRecoveryKey)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(key.raw[:], []byte("\x00\x01\x02\x03\x04\x05\x06\x07\x08\x09\x0a\x0b\x0c\x0d\x0e\x0f\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1b\x1c\x1d\x1e\x1f")) {
		t.Fatalf("the text form decodes to the wrong key")
	}
	if got := key.Text(); got != vectorRecoveryKey {
		t.Fatalf("Text() = %q", got)
	}
	keys := vectorKeys(t)
	for _, v := range fixedVectors {
		t.Run(v.name, func(t *testing.T) {
			if got := keys.BlobID([]byte(v.plain)); got != v.id {
				t.Fatalf("BlobID = %s", got)
			}
			sealed, err := hex.DecodeString(v.sealed)
			if err != nil {
				t.Fatal(err)
			}
			plain, err := keys.Open(v.id, sealed)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			if string(plain) != v.plain {
				t.Fatalf("Open = %q", plain)
			}
			// Nonces are random, so a fresh seal can only match in its header
			// and its ID, and has to open like the fixed one did.
			id, fresh, err := keys.Seal([]byte(v.plain))
			if err != nil {
				t.Fatal(err)
			}
			if id != v.id || !bytes.Equal(fresh[:headerSize], sealed[:headerSize]) {
				t.Fatalf("a fresh seal differs: id %s header %x", id, fresh[:headerSize])
			}
		})
	}
}

func TestSealOpenRoundTrip(t *testing.T) {
	keys := newKeys(t)
	cases := map[string][]byte{
		"empty":      {},
		"one byte":   {0x42},
		"text":       []byte(strings.Repeat("문장 하나.\n", 5000)),
		"random":     randomBytes(64 << 10),
		"largest":    randomBytes(MaxPlaintext),
		"compresses": make([]byte, MaxPlaintext),
	}
	for name, plain := range cases {
		t.Run(name, func(t *testing.T) {
			id, sealed, err := keys.Seal(plain)
			if err != nil {
				t.Fatal(err)
			}
			if len(sealed) > MaxBlobBytes {
				t.Fatalf("sealed %d bytes, the site takes %d", len(sealed), MaxBlobBytes)
			}
			got, err := keys.Open(id, sealed)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, plain) {
				t.Fatal("round trip changed the plaintext")
			}
		})
	}
}

// The largest plaintext has to fit the site's limit to the byte, or a part that
// was split at MaxPlaintext fails to upload.
func TestLargestPlaintextFillsABlobExactly(t *testing.T) {
	keys := newKeys(t)
	_, sealed, err := keys.Seal(randomBytes(MaxPlaintext))
	if err != nil {
		t.Fatal(err)
	}
	if len(sealed) != MaxBlobBytes {
		t.Fatalf("sealed %d bytes, want exactly %d", len(sealed), MaxBlobBytes)
	}
	if _, _, err := keys.Seal(randomBytes(MaxPlaintext + 1)); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("one byte over: %v", err)
	}
}

func TestSealCompressesOnlyWhenItHelps(t *testing.T) {
	keys := newKeys(t)
	text := []byte(strings.Repeat("비 내리는 밤이었다.\n", 2000))
	_, sealed, err := keys.Seal(text)
	if err != nil {
		t.Fatal(err)
	}
	if len(sealed) >= len(text)/2 {
		t.Fatalf("text sealed to %d of %d bytes", len(sealed), len(text))
	}
	picture := randomBytes(4096)
	_, sealed, err = keys.Seal(picture)
	if err != nil {
		t.Fatal(err)
	}
	if want := len(picture) + Overhead; len(sealed) != want {
		t.Fatalf("incompressible data sealed to %d bytes, want %d", len(sealed), want)
	}
}

func TestSealedBlobShowsNothing(t *testing.T) {
	keys := newKeys(t)
	marker := []byte("HONG-GIL-DONG-OPENS-THE-DOOR")
	for _, plain := range [][]byte{marker, bytes.Repeat(marker, 200)} {
		id, sealed, err := keys.Seal(plain)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(sealed, marker[:8]) {
			t.Fatal("plaintext is visible in the sealed blob")
		}
		_, again, err := keys.Seal(plain)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Equal(sealed, again) {
			t.Fatal("two seals reused a nonce")
		}
		if opened, err := keys.Open(id, again); err != nil || !bytes.Equal(opened, plain) {
			t.Fatalf("second seal: %v", err)
		}
	}
}

// Every byte of a sealed blob is covered: no single flip may yield a plaintext,
// and the region it falls in decides which failure the writer is told about.
func TestTamperingIsDetected(t *testing.T) {
	keys := newKeys(t)
	plain := []byte(strings.Repeat("고칠 수 없는 문장.", 8))
	id, sealed, err := keys.Seal(plain)
	if err != nil {
		t.Fatal(err)
	}
	versionAt := len(magic)
	checkAt := versionAt + 2
	for i := range sealed {
		changed := bytes.Clone(sealed)
		changed[i] ^= 0x01
		got, err := keys.Open(id, changed)
		if got != nil {
			t.Fatalf("byte %d flipped: returned a plaintext", i)
		}
		want := ErrCorrupt
		switch {
		case i == versionAt: // version 1 becomes 0
		case i >= checkAt && i < headerSize:
			want = ErrWrongKey
		}
		if !errors.Is(err, want) {
			t.Fatalf("byte %d flipped: %v, want %v", i, err, want)
		}
	}
	for n := range sealed {
		if got, err := keys.Open(id, sealed[:n]); got != nil || !errors.Is(err, ErrCorrupt) {
			t.Fatalf("truncated to %d: %v", n, err)
		}
	}
}

func TestNewerFormatIsRefusedBeforeDecrypting(t *testing.T) {
	keys := newKeys(t)
	id, sealed, err := keys.Seal([]byte("본문"))
	if err != nil {
		t.Fatal(err)
	}
	keys.aead = refusingAEAD{t}

	newer := bytes.Clone(sealed)
	newer[len(magic)] = FormatVersion + 1
	if _, err := keys.Open(id, newer); !errors.Is(err, ErrNewerFormat) {
		t.Fatalf("newer version: %v", err)
	}
}

// refusingAEAD fails the test if decryption is reached at all.
type refusingAEAD struct{ t *testing.T }

func (a refusingAEAD) NonceSize() int { return 0 }
func (a refusingAEAD) Overhead() int  { return gcmExtra }
func (a refusingAEAD) Seal(_, _, _, _ []byte) []byte {
	a.t.Fatal("reached encryption")
	return nil
}
func (a refusingAEAD) Open(_, _, _, _ []byte) ([]byte, error) {
	a.t.Fatal("reached decryption")
	return nil, nil
}

var _ cipher.AEAD = refusingAEAD{}

func TestAnotherKeyIsToldApartBeforeDecrypting(t *testing.T) {
	mine, other := newKeys(t), newKeys(t)
	id, sealed, err := mine.Seal([]byte("내 원고"))
	if err != nil {
		t.Fatal(err)
	}
	other.aead = refusingAEAD{t}
	if got, err := other.Open(id, sealed); got != nil || !errors.Is(err, ErrWrongKey) {
		t.Fatalf("Open under another key: %q, %v", got, err)
	}
}

// A blob opened under another blob's name is the server, or a mix-up, handing
// back the wrong part.
func TestABlobCannotAnswerForAnother(t *testing.T) {
	keys := newKeys(t)
	idA, sealedA, err := keys.Seal([]byte("1화"))
	if err != nil {
		t.Fatal(err)
	}
	idB, sealedB, err := keys.Seal([]byte("2화"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := keys.Open(idB, sealedA); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("A under B's name: %v", err)
	}
	if _, err := keys.Open(idA, sealedB); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("B under A's name: %v", err)
	}
}

// Past the tag, only a key holder could have made these. They are still
// refused: Open's promise is that the result is what the ID names.
func TestOpenChecksWhatTheKeyHolderSealed(t *testing.T) {
	keys := newKeys(t)

	t.Run("plaintext under another plaintext's ID", func(t *testing.T) {
		id := keys.BlobID([]byte("다른 내용"))
		if _, err := keys.Open(id, keys.seal(id, 0, []byte("이 내용"))); !errors.Is(err, ErrCorrupt) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("deflated flag on a body that is not", func(t *testing.T) {
		id := keys.BlobID([]byte("x"))
		if _, err := keys.Open(id, keys.seal(id, flagDeflate, []byte{0xff, 0xff, 0xff})); !errors.Is(err, ErrCorrupt) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("inflates past the largest plaintext", func(t *testing.T) {
		huge := make([]byte, MaxPlaintext+1)
		body, flags, err := pack(huge)
		if err != nil || flags != flagDeflate {
			t.Fatalf("pack: %v, flags %d", err, flags)
		}
		id := keys.BlobID(huge)
		if _, err := keys.Open(id, keys.seal(id, flags, body)); !errors.Is(err, ErrCorrupt) {
			t.Fatalf("got %v", err)
		}
	})
}

// Deflate reaches about 1000:1, so a 4MiB blob could otherwise ask for 4GiB. The
// refusal alone proves nothing: reading it all and then refusing would pass.
func TestInflatingStopsAtTheLimit(t *testing.T) {
	body, flags, err := pack(make([]byte, 16*MaxPlaintext))
	if err != nil || flags != flagDeflate {
		t.Fatalf("pack: %v, flags %d", err, flags)
	}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, err = unpack(body, flags)
	runtime.ReadMemStats(&after)
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("got %v", err)
	}
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > uint64(8*MaxPlaintext) {
		t.Fatalf("allocated %d bytes refusing a blob of %d", allocated, len(body))
	}
}

func TestRecoveryKeyText(t *testing.T) {
	key := NewRecoveryKey()
	text := key.Text()
	if len(text) != 69 || strings.Count(text, "-") != 13 {
		t.Fatalf("Text() = %d characters, %d groups", len(text), strings.Count(text, "-")+1)
	}
	if strings.ContainsAny(text, "ILOU") || text != strings.ToUpper(text) {
		t.Fatal("Text() uses a character the alphabet leaves out")
	}
	if NewRecoveryKey().Text() == text {
		t.Fatal("two new keys were the same")
	}

	bare := strings.ReplaceAll(text, "-", "")
	spaced := strings.ReplaceAll(text, "-", " ")
	forms := map[string]string{
		"as shown":    text,
		"lower case":  strings.ToLower(text),
		"no hyphens":  bare,
		"spaces":      spaced,
		"line breaks": "  " + bare[:28] + "\r\n" + bare[28:] + "\n",
		"look-alikes": strings.NewReplacer("0", "O", "1", "l").Replace(text),
	}
	for name, form := range forms {
		got, err := ParseRecoveryKey(form)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got.Text() != text {
			t.Fatalf("%s: parsed to a different key", name)
		}
	}
}

func TestMistypedRecoveryKeysAreRefused(t *testing.T) {
	const alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	text := strings.ReplaceAll(vectorRecoveryKey, "-", "")
	for i := range text {
		for _, c := range alphabet {
			if byte(c) == text[i] {
				continue
			}
			typo := text[:i] + string(c) + text[i+1:]
			if _, err := ParseRecoveryKey(typo); !errors.Is(err, ErrBadRecoveryKey) {
				t.Fatalf("typo at %d (%c): %v", i, c, err)
			}
		}
		if i+1 < len(text) && text[i] != text[i+1] {
			swapped := text[:i] + text[i+1:i+2] + text[i:i+1] + text[i+2:]
			if _, err := ParseRecoveryKey(swapped); !errors.Is(err, ErrBadRecoveryKey) {
				t.Fatalf("swap at %d: %v", i, err)
			}
		}
	}
	for _, junk := range []string{"", "-", "not a key", text[:55], text + "0", text + "00", "U" + text[1:]} {
		if _, err := ParseRecoveryKey(junk); !errors.Is(err, ErrBadRecoveryKey) {
			t.Fatalf("%q: %v", junk, err)
		}
	}
}

func TestParseErrorDoesNotRepeatTheInput(t *testing.T) {
	typo := "1" + vectorRecoveryKey[1:]
	_, err := ParseRecoveryKey(typo)
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if strings.Contains(err.Error(), typo[:9]) {
		t.Fatalf("error carries the key: %v", err)
	}
}

func TestKeysDoNotPrint(t *testing.T) {
	key, err := ParseRecoveryKey(vectorRecoveryKey)
	if err != nil {
		t.Fatal(err)
	}
	keys := vectorKeys(t)
	verbs := []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d", "%o", "%b", "%10.3v", "%+q"}
	for _, verb := range verbs {
		for name, value := range map[string]any{"key": key, "key pointer": &key, "keys": keys, "keys value": *keys} {
			if got := fmt.Sprintf(verb, value); got != redacted {
				t.Errorf("%s with %s printed %q", name, verb, got)
			}
		}
	}

	type holder struct {
		Exported RecoveryKey
		hidden   RecoveryKey
		keys     *Keys
	}
	nested := holder{Exported: key, hidden: key, keys: keys}
	for _, verb := range []string{"%v", "%+v", "%#v", "%d", "%x"} {
		got := fmt.Sprintf(verb, nested)
		for _, leak := range []string{vectorRecoveryKey, "0 1 2 3", "0x0 0x1", "000102", "0, 1, 2"} {
			if strings.Contains(got, leak) {
				t.Errorf("a key inside a struct printed with %s: %q", verb, got)
			}
		}
	}

	out, err := json.Marshal(map[string]any{"key": key, "keys": keys, "nested": nested})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), vectorRecoveryKey) || strings.Contains(string(out), "AAECAwQ") {
		t.Fatalf("JSON carries key material: %s", out)
	}
}
