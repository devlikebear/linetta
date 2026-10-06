package settings

import (
	"errors"
	"sync"
	"testing"
)

const webSearchAPIKeySecretName = "web_search.api_key"

// mcpTokenSecretName holds the bearer token external MCP clients present to
// the local server. Kept in the secret store, never in settings.json.
const mcpTokenSecretName = "mcp.token"

func providerAPIKeySecretName(provider string) string {
	return "provider." + provider + ".api_key"
}

// SecretStore persists credentials outside settings.json.
type SecretStore interface {
	Get(name string) (value string, ok bool, err error)
	Exists(name string) (ok bool, err error)
	Set(name, value string) error
	Delete(name string) error
}

// defaultSecretStore is the backend a Store gets when the caller names none:
// the OS credential store (platformSecretStore) in the app, and never that
// inside a test binary.
//
// The OS store is process-global and not scoped by LINETTA_HOME, so a test
// that reaches it against a t.TempDir() home still reads and writes the
// developer's real login keychain: `go test` left the app's own mcp.token
// item behind, and a differently-hashed test binary reading it back blocked on
// a keychain access prompt until the test timeout. Under test the default is
// therefore the no-backend store every Linux build already has, which also
// makes an un-injected test behave the same on every OS. A test that wants
// secrets to persist passes NewMemorySecretStore() explicitly.
func defaultSecretStore() SecretStore {
	if testing.Testing() {
		return unsupportedSecretStore{}
	}
	return platformSecretStore()
}

// NewMemorySecretStore returns an in-memory secret backend for tests.
func NewMemorySecretStore() SecretStore {
	return &memorySecretStore{data: map[string]string{}}
}

type memorySecretStore struct {
	mu   sync.Mutex
	data map[string]string
}

func (m *memorySecretStore) Get(name string) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.data[name]
	return v, ok, nil
}

func (m *memorySecretStore) Exists(name string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.data[name]
	return ok, nil
}

func (m *memorySecretStore) Set(name, value string) error {
	if value == "" {
		return errors.New("settings: refusing to store empty secret")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[name] = value
	return nil
}

func (m *memorySecretStore) Delete(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, name)
	return nil
}
