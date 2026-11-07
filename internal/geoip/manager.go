package geoip

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/oschwald/maxminddb-golang"
	log "github.com/sirupsen/logrus"
)

const (
	// DefaultDBRelPath is the standard local caching path for the Merged-IP database.
	DefaultDBRelPath = "data/geoip/Merged-IP.mmdb"
	// DefaultDownloadURL points to the latest release of NetworkCats/Merged-IP-Data.
	DefaultDownloadURL = "https://github.com/NetworkCats/Merged-IP-Data/releases/latest/download/Merged-IP.mmdb"
	// DefaultUpdateInterval is 24 hours between automated checks.
	DefaultUpdateInterval = 24 * time.Hour
)

var (
	defaultManagerInstance *Manager
	defaultManagerOnce     sync.Once
)

// Manager manages the lifecycle of the in-memory Merged-IP database reader,
// supporting dual-buffered atomic hot-reloading and background updates.
type Manager struct {
	mu           sync.RWMutex
	reader       *maxminddb.Reader
	dbPath       string
	downloadURL  string
	lastLoadedAt time.Time
	isUpdating   bool
	closeCh      chan struct{}
}

// Default returns the process-wide GeoIP Manager singleton.
func Default() *Manager {
	defaultManagerOnce.Do(func() {
		defaultManagerInstance = NewManager(Config{
			DBPath:         ResolveDBPath(""),
			AutoUpdate:     true,
			UpdateInterval: DefaultUpdateInterval,
			DownloadURL:    DefaultDownloadURL,
		})
	})
	return defaultManagerInstance
}

// NewManager initializes a GeoIP manager with the given configuration.
// It tries to open an existing mmdb database on disk; if missing, it operates
// gracefully in degraded mode without blocking or panicking.
func NewManager(cfg Config) *Manager {
	if cfg.DBPath == "" {
		cfg.DBPath = ResolveDBPath("")
	}
	if cfg.DownloadURL == "" {
		cfg.DownloadURL = DefaultDownloadURL
	}
	if cfg.UpdateInterval <= 0 {
		cfg.UpdateInterval = DefaultUpdateInterval
	}

	m := &Manager{
		dbPath:      cfg.DBPath,
		downloadURL: cfg.DownloadURL,
		closeCh:     make(chan struct{}),
	}

	// Try loading existing file from disk immediately
	if _, err := os.Stat(cfg.DBPath); err == nil {
		if errLoad := m.loadReader(cfg.DBPath); errLoad != nil {
			log.WithError(errLoad).Warnf("geoip: failed to load existing database at %s", cfg.DBPath)
		} else {
			log.Infof("geoip: loaded Merged-IP database from %s", cfg.DBPath)
		}
	} else {
		log.Infof("geoip: local mmdb not found at %s; will download in background", cfg.DBPath)
	}

	// Start auto-updater if enabled
	if cfg.AutoUpdate {
		go m.autoUpdateLoop(cfg.UpdateInterval)
	}

	return m
}

// ResolveDBPath resolves the database file path using env, working dir, and known system locations.
func ResolveDBPath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if envPath := os.Getenv("CPA_GEOIP_DB_PATH"); envPath != "" {
		return envPath
	}

	// 1. Current working directory ./data/geoip/Merged-IP.mmdb
	localPath := filepath.Join("data", "geoip", "Merged-IP.mmdb")
	if _, err := os.Stat(localPath); err == nil {
		return localPath
	}

	// 2. Standard system container path /CLIProxyAPI/data/geoip/Merged-IP.mmdb
	const containerPath = "/CLIProxyAPI/data/geoip/Merged-IP.mmdb"
	if _, err := os.Stat(containerPath); err == nil {
		return containerPath
	}

	// 3. Host production path /opt/cliproxyapi/data/geoip/Merged-IP.mmdb
	const hostPath = "/opt/cliproxyapi/data/geoip/Merged-IP.mmdb"
	if _, err := os.Stat(hostPath); err == nil {
		return hostPath
	}

	return localPath
}

// Lookup resolves the geographical and network metadata for a given IP address.
// It is guaranteed to be panic-free, thread-safe, and pure in-memory (no disk IO).
func (m *Manager) Lookup(rawAddr string) *Location {
	if m == nil {
		return nil
	}

	clean := strings.TrimSpace(rawAddr)
	if clean == "" {
		return nil
	}

	// Strip port if present
	host, _, err := net.SplitHostPort(clean)
	if err == nil && host != "" {
		clean = host
	}
	clean = strings.Trim(clean, "[]")
	ip := net.ParseIP(clean)

	// 1. Check special / internal / loopback / Docker / Tailscale ranges first
	if special := MatchSpecialIP(ip, clean); special != nil {
		return special
	}

	// If unparseable as an IP, return nil
	if ip == nil {
		return nil
	}

	// 2. Query in-memory MMDB reader under read lock
	m.mu.RLock()
	reader := m.reader
	m.mu.RUnlock()

	if reader == nil {
		// Fallback when database is not yet downloaded
		return &Location{
			Country:     "未知",
			CountryCode: "UN",
			Formatted:   "未知地域",
		}
	}

	var rec mmdbRecord
	if errLookup := reader.Lookup(ip, &rec); errLookup != nil {
		return &Location{
			Country:     "未知",
			CountryCode: "UN",
			Formatted:   "未知地域",
		}
	}

	// 3. Extract best language fields (Chinese priority, English fallback)
	loc := &Location{
		CountryCode: rec.Country.IsoCode,
		ASN:         rec.ASN.AutonomousSystemNumber,
		ASNOrg:      rec.ASN.AutonomousSystemOrganization,
		IsProxy:     rec.Proxy.IsProxy || rec.Proxy.IsAnonymous,
		IsVPN:       rec.Proxy.IsVPN,
		IsTor:       rec.Proxy.IsTor,
	}

	// Country name
	if c, ok := rec.Country.Names["zh-CN"]; ok && c != "" {
		loc.Country = c
	} else if c, ok := rec.Country.Names["en"]; ok && c != "" {
		loc.Country = c
	} else if rc, ok := rec.RegisteredCountry.Names["zh-CN"]; ok && rc != "" {
		loc.Country = rc
	} else if rc, ok := rec.RegisteredCountry.Names["en"]; ok && rc != "" {
		loc.Country = rc
	}
	if loc.CountryCode == "" && rec.RegisteredCountry.IsoCode != "" {
		loc.CountryCode = rec.RegisteredCountry.IsoCode
	}

	// Subdivision / Province
	if len(rec.Subdivisions) > 0 {
		if r, ok := rec.Subdivisions[0].Names["zh-CN"]; ok && r != "" {
			loc.Region = r
		} else if r, ok := rec.Subdivisions[0].Names["en"]; ok && r != "" {
			loc.Region = r
		}
	}

	// City
	if ci, ok := rec.City.Names["zh-CN"]; ok && ci != "" {
		loc.City = ci
	} else if ci, ok := rec.City.Names["en"]; ok && ci != "" {
		loc.City = ci
	}

	loc.Formatted = loc.Format()
	return loc
}

// loadReader loads and verifies an mmdb file from disk, performing an atomic pointer swap.
func (m *Manager) loadReader(path string) error {
	newReader, err := maxminddb.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open mmdb %s: %w", path, err)
	}

	// Verification: perform test lookup on known IP to ensure schema compatibility
	var testRec mmdbRecord
	if errTest := newReader.Lookup(net.ParseIP("8.8.8.8"), &testRec); errTest != nil {
		_ = newReader.Close()
		return fmt.Errorf("integrity check failed on %s: %w", path, errTest)
	}

	// Atomic hot-swap under write lock
	m.mu.Lock()
	oldReader := m.reader
	m.reader = newReader
	m.lastLoadedAt = time.Now()
	m.mu.Unlock()

	// Graceful delayed close of old reader to prevent races with ongoing in-flight queries
	if oldReader != nil {
		go func(r *maxminddb.Reader) {
			time.Sleep(5 * time.Second)
			_ = r.Close()
		}(oldReader)
	}

	return nil
}

// DownloadAndReload fetches the latest Merged-IP.mmdb, verifies it, and performs hot-swap.
func (m *Manager) DownloadAndReload(ctx context.Context) error {
	m.mu.Lock()
	if m.isUpdating {
		m.mu.Unlock()
		return fmt.Errorf("geoip: update already in progress")
	}
	m.isUpdating = true
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		m.isUpdating = false
		m.mu.Unlock()
	}()

	targetDir := filepath.Dir(m.dbPath)
	if errDir := os.MkdirAll(targetDir, 0755); errDir != nil {
		return fmt.Errorf("failed to create directory %s: %w", targetDir, errDir)
	}

	tmpFile := m.dbPath + fmt.Sprintf(".download-%d.tmp", time.Now().UnixNano())
	log.Infof("geoip: downloading latest Merged-IP.mmdb from %s", m.downloadURL)

	req, errReq := http.NewRequestWithContext(ctx, http.MethodGet, m.downloadURL, nil)
	if errReq != nil {
		return fmt.Errorf("failed to create request: %w", errReq)
	}
	req.Header.Set("User-Agent", "CLIProxyAPI-GeoIP-Updater/1.0")

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, errDo := client.Do(req)
	if errDo != nil {
		return fmt.Errorf("failed to download database: %w", errDo)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned non-200 status: %d", resp.StatusCode)
	}

	f, errCreate := os.Create(tmpFile)
	if errCreate != nil {
		return fmt.Errorf("failed to create temporary file %s: %w", tmpFile, errCreate)
	}

	written, errCopy := io.Copy(f, resp.Body)
	_ = f.Close()
	if errCopy != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to write temporary file: %w", errCopy)
	}

	if written < 10*1024*1024 { // Valid database should be at least 10MB
		_ = os.Remove(tmpFile)
		return fmt.Errorf("downloaded file too small (%d bytes), aborting update", written)
	}

	// Verify temp file opens correctly and parses
	verifyReader, errVerify := maxminddb.Open(tmpFile)
	if errVerify != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("downloaded file failed mmdb parse validation: %w", errVerify)
	}
	var testRec mmdbRecord
	if errTest := verifyReader.Lookup(net.ParseIP("8.8.8.8"), &testRec); errTest != nil {
		_ = verifyReader.Close()
		_ = os.Remove(tmpFile)
		return fmt.Errorf("downloaded file failed lookup test: %w", errTest)
	}
	_ = verifyReader.Close()

	// Atomic rename to final target path
	if errRename := os.Rename(tmpFile, m.dbPath); errRename != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to move %s to %s: %w", tmpFile, m.dbPath, errRename)
	}

	// Hot reload new reader
	if errLoad := m.loadReader(m.dbPath); errLoad != nil {
		return fmt.Errorf("failed to reload newly installed database: %w", errLoad)
	}

	log.Infof("geoip: successfully downloaded and hot-swapped Merged-IP database (%d MB)", written/(1024*1024))
	return nil
}

// autoUpdateLoop executes periodic checks and updates.
func (m *Manager) autoUpdateLoop(interval time.Duration) {
	// First check: if reader is nil, attempt download shortly after boot
	m.mu.RLock()
	hasReader := (m.reader != nil)
	m.mu.RUnlock()

	if !hasReader {
		time.Sleep(3 * time.Second)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		if err := m.DownloadAndReload(ctx); err != nil {
			log.WithError(err).Warn("geoip: initial background database download failed")
		}
		cancel()
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-m.closeCh:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			if err := m.DownloadAndReload(ctx); err != nil {
				log.WithError(err).Warn("geoip: scheduled database update failed")
			}
			cancel()
		}
	}
}

// Close gracefully stops the auto-updater and closes the active database reader.
func (m *Manager) Close() error {
	if m == nil {
		return nil
	}
	close(m.closeCh)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.reader != nil {
		err := m.reader.Close()
		m.reader = nil
		return err
	}
	return nil
}
