package vault

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/linetta/engine/internal/vaultbundle"
)

func reason(code string, err error) error {
	return &rpc.ReasonError{Reason: code, Err: err}
}

// classify turns a failure into a reason code the pane can explain. A
// ReasonError passes through, and anything that is not about the vault (a full
// disk, a database error) stays an internal error with its own message.
func classify(err error) error {
	if err == nil {
		return nil
	}
	var re *rpc.ReasonError
	if errors.As(err, &re) || errors.Is(err, context.Canceled) {
		return err
	}
	for sentinel, code := range map[error]string{
		ErrBadRecoveryKey:          rpc.ReasonVaultBadKey,
		ErrWrongKey:                rpc.ReasonVaultWrongKey,
		ErrCorrupt:                 rpc.ReasonVaultDamaged,
		ErrNewerFormat:             rpc.ReasonVaultNewerVersion,
		ErrTooLarge:                rpc.ReasonVaultTooLarge,
		vaultbundle.ErrCorrupt:     rpc.ReasonVaultDamaged,
		vaultbundle.ErrNewerFormat: rpc.ReasonVaultNewerVersion,
		vaultbundle.ErrNewerSchema: rpc.ReasonVaultNewerVersion,
		settings.ErrVaultKeyExists: rpc.ReasonVaultKeyExists,
	} {
		if errors.Is(err, sentinel) {
			return reason(code, err)
		}
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return reason(reasonForAPIError(apiErr), err)
	}
	var netErr net.Error
	var dnsErr *net.DNSError
	var opErr *net.OpError
	switch {
	case isTimeout(err):
		return reason(rpc.ReasonVaultTimeout, err)
	case errors.As(err, &netErr), errors.As(err, &dnsErr), errors.As(err, &opErr):
		return reason(rpc.ReasonVaultUnreachable, err)
	}
	return err
}

func reasonForAPIError(e *APIError) string {
	switch e.Code {
	case CodeUnauthorized:
		return rpc.ReasonVaultUnauthorized
	case CodeNotEnabled:
		return rpc.ReasonVaultNotEnabled
	case CodeTermsOutdated:
		return rpc.ReasonVaultTermsOutdated
	case CodeQuotaExceeded:
		return rpc.ReasonVaultQuotaExceeded
	case CodeConflict:
		return rpc.ReasonVaultConflict
	case CodeTooLarge:
		return rpc.ReasonVaultTooLarge
	case CodeRateLimited:
		return rpc.ReasonVaultRateLimited
	case CodeNotFound:
		return rpc.ReasonVaultItemNotFound
	}
	switch {
	case e.Status == http.StatusUnauthorized:
		return rpc.ReasonVaultUnauthorized
	case e.Status == http.StatusRequestTimeout || e.Status == http.StatusGatewayTimeout:
		return rpc.ReasonVaultTimeout
	case e.Status >= 500:
		return rpc.ReasonVaultUnavailable
	}
	return rpc.ReasonVaultRejected
}

func isCode(err error, code string) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Code == code
}

func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "client.timeout exceeded")
}
