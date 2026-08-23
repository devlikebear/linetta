//go:build !mobile

package mcphost

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"syscall"
)

// authMiddleware gates the MCP endpoint. Two independent checks:
//
//  1. A bearer token, compared in constant time. This is what actually
//     authorizes the caller.
//  2. Origin/Host validation. Without it any web page the writer visits could
//     POST to 127.0.0.1 and drive their manuscript — the DNS-rebinding case
//     the MCP spec calls out for HTTP transports. A browser always sends
//     Origin on cross-origin requests; a legitimate MCP client sends none.
func authMiddleware(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !originAllowed(r.Header.Get("Origin")) {
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}
		if !hostAllowed(r.Host) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if !tokenMatches(token, r.Header.Get("Authorization")) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="linetta"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// tokenMatches accepts only "Bearer <token>" with an exact, constant-time match.
func tokenMatches(want, header string) bool {
	if want == "" {
		return false
	}
	const prefix = "Bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return false
	}
	got := strings.TrimSpace(header[len(prefix):])
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// originAllowed permits a missing Origin (native MCP clients send none) and
// loopback origins; everything else is rejected.
func originAllowed(origin string) bool {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return isLoopbackHost(u.Hostname())
}

// hostAllowed rejects a Host header pointing anywhere but loopback, so a
// rebound DNS name cannot be used to reach the server.
func hostAllowed(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" {
		return false
	}
	name, _, err := net.SplitHostPort(host)
	if err != nil {
		name = host
	}
	return isLoopbackHost(name)
}

func isLoopbackHost(name string) bool {
	name = strings.TrimSpace(strings.Trim(name, "[]"))
	if name == "" {
		return false
	}
	if strings.EqualFold(name, "localhost") {
		return true
	}
	ip := net.ParseIP(name)
	return ip != nil && ip.IsLoopback()
}

// isAddrInUse reports whether err is the OS "address already in use" error.
// Windows uses WSAEADDRINUSE (10048) rather than the POSIX constant.
func isAddrInUse(err error) bool {
	if errors.Is(err, syscall.EADDRINUSE) {
		return true
	}
	var errno syscall.Errno
	if errors.As(err, &errno) && uintptr(errno) == 10048 {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "address already in use") ||
		strings.Contains(strings.ToLower(err.Error()), "only one usage of each socket address")
}

func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "mcphost: "+format+"\n", args...)
}
