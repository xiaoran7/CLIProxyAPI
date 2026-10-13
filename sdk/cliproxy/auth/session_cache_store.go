package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// SessionCacheStore persists runtime session-to-auth bindings across process restarts.
type SessionCacheStore interface {
	Load(context.Context) ([]SessionCacheRecord, error)
	Save(context.Context, []SessionCacheRecord) error
}

type sessionCacheFile struct {
	Version   int                  `json:"version"`
	UpdatedAt time.Time            `json:"updated_at"`
	Records   []SessionCacheRecord `json:"records"`
}

// FileSessionCacheStore persists session affinity records to a JSON file.
type FileSessionCacheStore struct {
	mu   sync.Mutex
	path string
}

// NewFileSessionCacheStore creates a file-backed session cache store at the given path.
func NewFileSessionCacheStore(path string) *FileSessionCacheStore {
	return &FileSessionCacheStore{
		path: path,
	}
}

// Load reads all valid session affinity records from disk.
func (s *FileSessionCacheStore) Load(ctx context.Context) ([]SessionCacheRecord, error) {
	if s == nil || s.path == "" {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read session cache file: %w", err)
	}

	var file sessionCacheFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("unmarshal session cache file: %w", err)
	}

	now := time.Now()
	valid := make([]SessionCacheRecord, 0, len(file.Records))
	for _, rec := range file.Records {
		if now.Before(rec.ExpiresAt) && rec.AuthID != "" && len(rec.Aliases) > 0 {
			valid = append(valid, rec)
		}
	}
	return valid, nil
}

// Save writes active session affinity records to disk atomically.
func (s *FileSessionCacheStore) Save(ctx context.Context, records []SessionCacheRecord) error {
	if s == nil || s.path == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	valid := make([]SessionCacheRecord, 0, len(records))
	for _, rec := range records {
		if now.Before(rec.ExpiresAt) && rec.AuthID != "" && len(rec.Aliases) > 0 {
			valid = append(valid, rec)
		}
	}

	file := sessionCacheFile{
		Version:   1,
		UpdatedAt: now,
		Records:   valid,
	}

	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session cache file: %w", err)
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create session cache dir: %w", err)
	}

	tmpFile := s.path + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0o600); err != nil {
		return fmt.Errorf("write temp session cache file: %w", err)
	}

	if err := os.Rename(tmpFile, s.path); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("atomic rename session cache file: %w", err)
	}

	log.Debugf("session_cache_store: persisted %d session affinity records to %s", len(valid), s.path)
	return nil
}
