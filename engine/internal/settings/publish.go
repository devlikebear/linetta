package settings

import (
	"os"
	"path/filepath"
	"strings"
)

// publishTokenFileName holds the publish token on platforms with no secure
// secret backend (Linux, as of today — see secrets_unsupported.go).
//
// Plaintext at 0600 is the same call mcp-token makes, for a similar reason:
// the token can do one thing — put this writer's own episodes on the site —
// and the writer can revoke it from their profile on the site at any time,
// while any process running as this user can already read library.db and the
// manuscripts themselves. Publishing is a shipping feature on Linux, and a
// connection that cannot be stored cannot be used at all.
const publishTokenFileName = "publish-token"

// PublishAccount is the Linetta Works account this device is connected to:
// the parts that are not secret, kept in settings.json. The token is not
// here; it is in the secret store.
type PublishAccount struct {
	SiteURL string `json:"site_url"`
	Handle  string `json:"handle"`
	PenName string `json:"pen_name"`
	// ConnectedAt is when the writer approved this device, and ConsentedAt
	// when they accepted, in the app, that published text leaves the device.
	ConnectedAt int64 `json:"connected_at"`
	ConsentedAt int64 `json:"consented_at"`
}

// PublishAccount returns the connected account, if any.
func (s *Store) PublishAccount() (PublishAccount, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cfg.PublishAccount == nil {
		return PublishAccount{}, false
	}
	return *s.cfg.PublishAccount, true
}

// SetPublishAccount stores the connected account, or forgets it when acc is
// nil. It is not reachable through settings.set: only the connect flow,
// having just been told by the site who approved it, writes it.
func (s *Store) SetPublishAccount(acc *PublishAccount) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.mu.RLock()
	next := s.cfg
	s.mu.RUnlock()
	if acc != nil {
		copied := *acc
		next.PublishAccount = &copied
	} else {
		next.PublishAccount = nil
	}
	if err := s.persist(next); err != nil {
		return err
	}
	s.mu.Lock()
	s.cfg.PublishAccount = next.PublishAccount
	s.mu.Unlock()
	return nil
}

func (s *Store) publishTokenPath() string {
	return filepath.Join(s.dir, publishTokenFileName)
}

// PublishToken returns the publish token, or "" when this device is not
// connected.
func (s *Store) PublishToken() string {
	if secret, ok, err := s.secrets.Get(publishTokenSecretName); err == nil && ok {
		return secret
	}
	raw, err := os.ReadFile(s.publishTokenPath())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

// PublishTokenExists reports whether a publish token is stored, without
// reading its value (on macOS a read can raise a Keychain prompt).
func (s *Store) PublishTokenExists() bool {
	if ok, err := s.secrets.Exists(publishTokenSecretName); err == nil && ok {
		return true
	}
	_, err := os.Stat(s.publishTokenPath())
	return err == nil
}

// SetPublishToken stores the publish token in the secret store, or in a 0600
// file where there is none (see publishTokenFileName).
func (s *Store) SetPublishToken(token string) error {
	if err := s.secrets.Set(publishTokenSecretName, token); err != nil {
		if writeErr := os.WriteFile(s.publishTokenPath(), []byte(token), 0o600); writeErr != nil {
			return err
		}
		// WriteFile keeps an existing file's mode; make sure it is 0600.
		return os.Chmod(s.publishTokenPath(), 0o600)
	}
	// A token in the secret store makes an older fallback file stale.
	_ = os.Remove(s.publishTokenPath())
	return nil
}

// DeletePublishToken removes the publish token from both possible places.
func (s *Store) DeletePublishToken() error {
	err := s.secrets.Delete(publishTokenSecretName)
	if rmErr := os.Remove(s.publishTokenPath()); rmErr != nil && !os.IsNotExist(rmErr) && err == nil {
		err = rmErr
	}
	return err
}
