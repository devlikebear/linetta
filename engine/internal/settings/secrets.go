package settings

import (
	"errors"
	"sync"
)

const webSearchAPIKeySecretName = "web_search.api_key"

func providerAPIKeySecretName(provider string) string {
	return "provider." + provider + ".api_key"
}

// SecretStore persists credentials outside settings.json.
type SecretStore interface {
	Get(name string) (value string, ok bool, err error)
	Set(name, value string) error
	Delete(name string) error
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
