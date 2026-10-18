package publish

import (
	"errors"

	"github.com/devlikebear/linetta/engine/internal/settings"
)

// Credentials is where a connection lives: the account (handle, pen name,
// site) in settings.json, the token in the OS secret store — the macOS
// Keychain or the Windows Credential Manager — or, on Linux, a 0600 file
// beside settings.json (see settings/publish.go for why that is acceptable).
type Credentials interface {
	Account() (settings.PublishAccount, bool)
	Token() string
	HasToken() bool
	Save(acc settings.PublishAccount, token string) error
	Clear() error
}

// NewCredentials keeps the connection in the app's settings store.
func NewCredentials(s *settings.Store) Credentials { return settingsCredentials{s: s} }

type settingsCredentials struct{ s *settings.Store }

func (c settingsCredentials) Account() (settings.PublishAccount, bool) {
	return c.s.PublishAccount()
}

func (c settingsCredentials) Token() string { return c.s.PublishToken() }

func (c settingsCredentials) HasToken() bool { return c.s.PublishTokenExists() }

// Save stores the token first and the account second, and takes the token
// back out if the account cannot be written: an account on disk with no token
// behind it would show "connected" for a device that cannot publish.
func (c settingsCredentials) Save(acc settings.PublishAccount, token string) error {
	if token == "" {
		return errors.New("publish: refusing to store an empty token")
	}
	if err := c.s.SetPublishToken(token); err != nil {
		return err
	}
	if err := c.s.SetPublishAccount(&acc); err != nil {
		_ = c.s.DeletePublishToken()
		return err
	}
	return nil
}

// Clear forgets the connection on this device. The site still lists the
// device until the writer removes it from their profile there.
func (c settingsCredentials) Clear() error {
	tokenErr := c.s.DeletePublishToken()
	if err := c.s.SetPublishAccount(nil); err != nil {
		return err
	}
	return tokenErr
}
