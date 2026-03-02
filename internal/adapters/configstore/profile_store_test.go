package configstore

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ChewX3D/wbcli/internal/app/ports"
)

func TestFileSessionStoreWritesMetadataOnlyWith0600Permissions(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	store := NewFileSessionStore(configPath)

	now := time.Date(2026, 2, 26, 12, 0, 0, 0, time.UTC)
	err := store.SaveSession(context.Background(), ports.SessionMetadata{
		Backend:    "os-keychain",
		APIKeyHint: "ab***yz",
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	fileData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	text := string(fileData)
	if strings.HasPrefix(strings.TrimSpace(text), "{") {
		t.Fatalf("expected yaml output, got json payload: %s", text)
	}
	if !strings.Contains(text, "schema_version: 2") {
		t.Fatalf("expected yaml schema_version entry, got: %s", text)
	}
	if strings.Contains(text, "api_secret") || strings.Contains(text, "secret") {
		t.Fatalf("config must not contain secret values, got: %s", text)
	}

	fileInfo, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if fileInfo.Mode().Perm() != 0o600 {
		t.Fatalf("expected config mode 0600, got %o", fileInfo.Mode().Perm())
	}

	session, found, err := store.GetSession(context.Background())
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if !found {
		t.Fatalf("expected session to exist")
	}
	if session.Backend != "os-keychain" {
		t.Fatalf("expected backend os-keychain, got %q", session.Backend)
	}
}

func TestFileSessionStoreReadsLegacyJSONConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	content := `{
  "schema_version": 2,
  "session": {
    "backend": "os-keychain",
    "api_key_hint": "ab***yz",
    "hedge_mode": true,
    "created_at": "2026-03-02T10:00:00Z",
    "updated_at": "2026-03-02T10:10:00Z"
  }
}
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	store := NewFileSessionStore(configPath)
	session, found, err := store.GetSession(context.Background())
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if !found {
		t.Fatalf("expected session to exist")
	}
	if session.HedgeMode == nil || !*session.HedgeMode {
		t.Fatalf("expected hedge_mode=true from legacy json config, got %#v", session.HedgeMode)
	}
}
