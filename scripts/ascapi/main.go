// Command ascapi makes one authenticated App Store Connect API request.
//
// Usage: ascapi METHOD PATH [bodyFile]
// Reads credentials from env: APP_STORE_CONNECT_KEY_ID, APP_STORE_CONNECT_ISSUER_ID,
// AUTH_KEY_PATH (path to the .p8). Prints the response body to stdout; exits 1 on
// HTTP >= 300 (after still printing the body, so callers can inspect errors).
package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	if len(os.Args) < 3 {
		fail("usage: ascapi METHOD PATH [bodyFile]")
	}
	method, path := os.Args[1], os.Args[2]
	var body io.Reader
	if len(os.Args) > 3 {
		b, err := os.ReadFile(os.Args[3])
		check(err)
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, "https://api.appstoreconnect.apple.com"+path, body)
	check(err)
	req.Header.Set("Authorization", "Bearer "+token())
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	check(err)
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	os.Stdout.Write(out)
	if resp.StatusCode >= 300 {
		fmt.Fprintf(os.Stderr, "\nascapi: HTTP %d\n", resp.StatusCode)
		os.Exit(1)
	}
}

func token() string {
	kid := must("APP_STORE_CONNECT_KEY_ID")
	iss := must("APP_STORE_CONNECT_ISSUER_ID")
	keyPath := expand(must("AUTH_KEY_PATH"))
	pemBytes, err := os.ReadFile(keyPath)
	check(err)
	blk, _ := pem.Decode(pemBytes)
	if blk == nil {
		fail("AUTH_KEY_PATH is not a PEM .p8 file")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(blk.Bytes)
	check(err)
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		fail("p8 key is not ECDSA")
	}

	now := time.Now().Unix()
	header, _ := json.Marshal(map[string]string{"alg": "ES256", "kid": kid, "typ": "JWT"})
	payload, _ := json.Marshal(map[string]any{
		"iss": iss, "iat": now, "exp": now + 1200, "aud": "appstoreconnect-v1",
	})
	signing := b64(header) + "." + b64(payload)
	digest := sha256.Sum256([]byte(signing))
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	check(err)
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return signing + "." + b64(sig)
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func expand(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return p
}

func must(name string) string {
	v := os.Getenv(name)
	if v == "" {
		fail("missing env: " + name)
	}
	return v
}

func check(err error) {
	if err != nil {
		fail(err.Error())
	}
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "ascapi: "+msg)
	os.Exit(2)
}
