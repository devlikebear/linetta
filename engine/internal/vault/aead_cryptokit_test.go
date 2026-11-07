//go:build cgo && darwin && cryptokit

package vault

import (
	"bytes"
	"crypto/rand"
	"testing"
)

// Run by scripts/test-cryptokit-aead.sh, which builds engine/cryptokit for
// macOS and links it in. A blob sealed on the desktop (Go) has to open on iOS
// (CryptoKit), and the other way round.

func pair(t *testing.T) (goSide, kit interface {
	Seal(dst, nonce, plaintext, additionalData []byte) []byte
	Open(dst, nonce, ciphertext, additionalData []byte) ([]byte, error)
}) {
	t.Helper()
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	g, err := goAEAD(key)
	if err != nil {
		t.Fatal(err)
	}
	k, err := cryptoKitAEAD(key)
	if err != nil {
		t.Fatal(err)
	}
	return g, k
}

func TestCryptoKitIsWhatIOSSealsWith(t *testing.T) {
	aead, err := newAEAD(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := aead.(cryptoKit); !ok {
		t.Fatalf("newAEAD made %T", aead)
	}
}

func TestCryptoKitAndGoOpenEachOthersBlobs(t *testing.T) {
	g, k := pair(t)
	header := []byte("LNVB-header")
	big := make([]byte, MaxPlaintext)
	_, _ = rand.Read(big)
	for name, plain := range map[string][]byte{"empty": {}, "short": []byte("숨겨진 첫 문장"), "largest": big} {
		ad := []byte("id-" + name)
		fromGo := g.Seal(bytes.Clone(header), nil, plain, ad)
		fromKit := k.Seal(bytes.Clone(header), nil, plain, ad)
		if len(fromGo) != len(fromKit) || !bytes.Equal(fromKit[:len(header)], header) {
			t.Fatalf("%s: layouts differ: %d vs %d", name, len(fromGo), len(fromKit))
		}
		if got, err := k.Open(nil, nil, fromGo[len(header):], ad); err != nil || !bytes.Equal(got, plain) {
			t.Fatalf("%s: CryptoKit could not open Go's blob: %v", name, err)
		}
		if got, err := g.Open(nil, nil, fromKit[len(header):], ad); err != nil || !bytes.Equal(got, plain) {
			t.Fatalf("%s: Go could not open CryptoKit's blob: %v", name, err)
		}
	}
}

func TestCryptoKitRefusesWhatGoRefuses(t *testing.T) {
	g, k := pair(t)
	sealed := g.Seal(nil, nil, []byte("원고"), []byte("id"))
	if _, err := k.Open(nil, nil, sealed, []byte("other id")); err == nil {
		t.Error("opened under the wrong additional data")
	}
	tampered := bytes.Clone(sealed)
	tampered[len(tampered)-1] ^= 1
	if _, err := k.Open(nil, nil, tampered, []byte("id")); err == nil {
		t.Error("opened a tampered blob")
	}
	if _, err := k.Open(nil, nil, sealed[:10], []byte("id")); err == nil {
		t.Error("opened a truncated blob")
	}
	_, other := pair(t)
	if _, err := other.Open(nil, nil, sealed, []byte("id")); err == nil {
		t.Error("opened with another key")
	}
}

// The whole path: a recovery key's working keys sealing on one side and
// opening on the other.
func TestBlobsCrossBetweenDesktopAndIOS(t *testing.T) {
	key := NewRecoveryKey()
	desktop := func() *Keys {
		newAEAD = goAEAD
		defer func() { newAEAD = cryptoKitAEAD }()
		keys, err := key.Derive()
		if err != nil {
			t.Fatal(err)
		}
		return keys
	}()
	ios, err := key.Derive()
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte("기기 사이를 오가는 원고")
	id, sealed, err := desktop.Seal(plain)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := ios.Open(id, sealed); err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("iOS could not open the desktop's blob: %v", err)
	}
	id, sealed, err = ios.Seal(plain)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := desktop.Open(id, sealed); err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("the desktop could not open iOS's blob: %v", err)
	}
}
