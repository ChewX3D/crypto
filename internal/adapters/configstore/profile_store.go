package configstore

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ChewX3D/wbcli/internal/app/ports"
)

const (
	configDirName          = ".wbcli"
	defaultConfigFileName  = "config.yaml"
	configSchemaVersionV2  = 2
	timestampLayoutRFC3339 = time.RFC3339Nano
)

type storedConfig struct {
	SchemaVersion int            `json:"schema_version"`
	Session       *storedSession `json:"session,omitempty"`
}

type storedSession struct {
	Backend    string `json:"backend,omitempty"`
	APIKeyHint string `json:"api_key_hint,omitempty"`
	HedgeMode  *bool  `json:"hedge_mode,omitempty"`
	CreatedAt  string `json:"created_at,omitempty"`
	UpdatedAt  string `json:"updated_at,omitempty"`
}

// FileSessionStore stores auth session metadata in local config file.
type FileSessionStore struct {
	path string
	mu   sync.Mutex
}

// NewDefaultSessionStore constructs session store at ~/.wbcli/config.yaml.
func NewDefaultSessionStore() (*FileSessionStore, error) {
	configPath, err := defaultConfigPath()
	if err != nil {
		return nil, err
	}

	return NewFileSessionStore(configPath), nil
}

// NewFileSessionStore constructs session store at custom path.
func NewFileSessionStore(path string) *FileSessionStore {
	return &FileSessionStore{path: path}
}

func defaultConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home directory: %w", err)
	}

	return filepath.Join(homeDir, configDirName, defaultConfigFileName), nil
}

// SaveSession creates or updates non-secret auth session metadata.
func (store *FileSessionStore) SaveSession(_ context.Context, session ports.SessionMetadata) error {
	store.mu.Lock()
	defer store.mu.Unlock()

	config, err := store.loadConfig()
	if err != nil {
		return err
	}

	config.Session = &storedSession{
		Backend:    session.Backend,
		APIKeyHint: session.APIKeyHint,
		HedgeMode:  copyBoolPtr(session.HedgeMode),
		CreatedAt:  session.CreatedAt.UTC().Format(timestampLayoutRFC3339),
		UpdatedAt:  session.UpdatedAt.UTC().Format(timestampLayoutRFC3339),
	}

	return store.saveConfig(config)
}

// GetSession returns current auth session metadata.
func (store *FileSessionStore) GetSession(_ context.Context) (ports.SessionMetadata, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	config, err := store.loadConfig()
	if err != nil {
		return ports.SessionMetadata{}, false, err
	}
	if config.Session == nil {
		return ports.SessionMetadata{}, false, nil
	}

	metadata, err := storedToSession(*config.Session)
	if err != nil {
		return ports.SessionMetadata{}, false, err
	}

	return metadata, true, nil
}

// ClearSession clears auth session metadata.
func (store *FileSessionStore) ClearSession(_ context.Context) error {
	store.mu.Lock()
	defer store.mu.Unlock()

	config, err := store.loadConfig()
	if err != nil {
		return err
	}
	config.Session = nil

	return store.saveConfig(config)
}

func (store *FileSessionStore) loadConfig() (storedConfig, error) {
	fileData, err := os.ReadFile(store.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return storedConfig{SchemaVersion: configSchemaVersionV2}, nil
		}

		return storedConfig{}, fmt.Errorf("read session config: %w", err)
	}

	if len(fileData) == 0 {
		return storedConfig{SchemaVersion: configSchemaVersionV2}, nil
	}

	config, err := decodeConfig(fileData)
	if err != nil {
		return storedConfig{}, fmt.Errorf("decode session config: %w", err)
	}
	if config.SchemaVersion == 0 {
		config.SchemaVersion = configSchemaVersionV2
	}

	return config, nil
}

func (store *FileSessionStore) saveConfig(config storedConfig) error {
	if config.SchemaVersion == 0 {
		config.SchemaVersion = configSchemaVersionV2
	}

	configDir := filepath.Dir(store.path)
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	tempFile, err := os.CreateTemp(configDir, "config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp config file: %w", err)
	}
	tempFilePath := tempFile.Name()
	defer os.Remove(tempFilePath)

	if err := tempFile.Chmod(0o600); err != nil {
		tempFile.Close()
		return fmt.Errorf("set temp config mode: %w", err)
	}

	encoded, err := encodeConfigYAML(config)
	if err != nil {
		tempFile.Close()
		return fmt.Errorf("encode session config: %w", err)
	}

	if _, err := tempFile.Write(encoded); err != nil {
		tempFile.Close()
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temp config: %w", err)
	}

	if err := os.Rename(tempFilePath, store.path); err != nil {
		return fmt.Errorf("replace config file: %w", err)
	}
	if err := os.Chmod(store.path, 0o600); err != nil {
		return fmt.Errorf("set config mode: %w", err)
	}

	return nil
}

func storedToSession(session storedSession) (ports.SessionMetadata, error) {
	createdAt, err := parseTimestamp(session.CreatedAt)
	if err != nil {
		return ports.SessionMetadata{}, fmt.Errorf("decode created_at: %w", err)
	}
	updatedAt, err := parseTimestamp(session.UpdatedAt)
	if err != nil {
		return ports.SessionMetadata{}, fmt.Errorf("decode updated_at: %w", err)
	}

	return ports.SessionMetadata{
		Backend:    session.Backend,
		APIKeyHint: session.APIKeyHint,
		HedgeMode:  copyBoolPtr(session.HedgeMode),
		CreatedAt:  createdAt,
		UpdatedAt:  updatedAt,
	}, nil
}

func parseTimestamp(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}

	parsed, err := time.Parse(timestampLayoutRFC3339, value)
	if err != nil {
		return time.Time{}, err
	}

	return parsed, nil
}

func decodeConfig(fileData []byte) (storedConfig, error) {
	var config storedConfig
	if err := json.Unmarshal(fileData, &config); err == nil {
		return config, nil
	}

	return decodeYAMLConfig(fileData)
}

func decodeYAMLConfig(fileData []byte) (storedConfig, error) {
	config := storedConfig{}
	var (
		inSession bool
		session   *storedSession
	)

	scanner := bufio.NewScanner(strings.NewReader(string(fileData)))
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		indent := len(line) - len(strings.TrimLeft(line, " "))
		if indent == 0 {
			inSession = false

			key, value, hasValue, err := splitYAMLKeyValue(trimmed)
			if err != nil {
				return storedConfig{}, fmt.Errorf("line %d: %w", lineNumber, err)
			}

			switch key {
			case "schema_version":
				if !hasValue {
					return storedConfig{}, fmt.Errorf("line %d: schema_version requires a value", lineNumber)
				}
				parsed, err := strconv.Atoi(value)
				if err != nil {
					return storedConfig{}, fmt.Errorf("line %d: parse schema_version: %w", lineNumber, err)
				}
				config.SchemaVersion = parsed
			case "session":
				if hasValue && strings.TrimSpace(value) != "" {
					return storedConfig{}, fmt.Errorf("line %d: session must be a map", lineNumber)
				}
				session = &storedSession{}
				config.Session = session
				inSession = true
			default:
				continue
			}

			continue
		}

		if !inSession || session == nil {
			continue
		}
		if indent < 2 {
			return storedConfig{}, fmt.Errorf("line %d: invalid indentation", lineNumber)
		}

		key, value, hasValue, err := splitYAMLKeyValue(strings.TrimSpace(line))
		if err != nil {
			return storedConfig{}, fmt.Errorf("line %d: %w", lineNumber, err)
		}
		if !hasValue {
			return storedConfig{}, fmt.Errorf("line %d: key %q requires a value", lineNumber, key)
		}

		switch key {
		case "backend":
			session.Backend = value
		case "api_key_hint":
			session.APIKeyHint = value
		case "created_at":
			session.CreatedAt = value
		case "updated_at":
			session.UpdatedAt = value
		case "hedge_mode":
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return storedConfig{}, fmt.Errorf("line %d: parse hedge_mode: %w", lineNumber, err)
			}
			session.HedgeMode = copyBoolPtr(&parsed)
		default:
			continue
		}
	}
	if err := scanner.Err(); err != nil {
		return storedConfig{}, err
	}

	return config, nil
}

func splitYAMLKeyValue(line string) (string, string, bool, error) {
	index := strings.IndexRune(line, ':')
	if index <= 0 {
		return "", "", false, fmt.Errorf("invalid key/value format")
	}

	key := strings.TrimSpace(line[:index])
	if key == "" {
		return "", "", false, fmt.Errorf("empty key")
	}

	rest := strings.TrimSpace(line[index+1:])
	if rest == "" {
		return key, "", false, nil
	}

	value, err := decodeYAMLScalar(rest)
	if err != nil {
		return "", "", false, err
	}

	return key, value, true, nil
}

func decodeYAMLScalar(value string) (string, error) {
	if len(value) < 2 {
		return value, nil
	}
	if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
		decoded, err := strconv.Unquote(value)
		if err != nil {
			return "", fmt.Errorf("unquote value: %w", err)
		}
		return decoded, nil
	}
	if strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") {
		return strings.ReplaceAll(value[1:len(value)-1], "''", "'"), nil
	}

	return value, nil
}

func encodeConfigYAML(config storedConfig) ([]byte, error) {
	builder := strings.Builder{}
	builder.WriteString("schema_version: ")
	builder.WriteString(strconv.Itoa(config.SchemaVersion))
	builder.WriteString("\n")

	if config.Session != nil {
		builder.WriteString("session:\n")
		writeSessionString(&builder, "backend", config.Session.Backend)
		writeSessionString(&builder, "api_key_hint", config.Session.APIKeyHint)
		if config.Session.HedgeMode != nil {
			builder.WriteString("  hedge_mode: ")
			if *config.Session.HedgeMode {
				builder.WriteString("true\n")
			} else {
				builder.WriteString("false\n")
			}
		}
		writeSessionString(&builder, "created_at", config.Session.CreatedAt)
		writeSessionString(&builder, "updated_at", config.Session.UpdatedAt)
	}

	return []byte(builder.String()), nil
}

func writeSessionString(builder *strings.Builder, key string, value string) {
	if value == "" {
		return
	}

	builder.WriteString("  ")
	builder.WriteString(key)
	builder.WriteString(": ")
	builder.WriteString(strconv.Quote(value))
	builder.WriteString("\n")
}

func copyBoolPtr(value *bool) *bool {
	if value == nil {
		return nil
	}

	allocated := *value
	return &allocated
}
