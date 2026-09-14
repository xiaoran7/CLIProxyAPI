package cliproxy

import (
	"context"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/discovery"
	log "github.com/sirupsen/logrus"
)

const defaultDiscoveryRefreshInterval = 15 * time.Second

type discoveryAdvertiserManager struct {
	mu              sync.Mutex
	advertiser      discovery.Advertiser
	enabled         bool
	lastSpec        discovery.ServiceSpec
	lastCfg         *config.Config
	lastPort        int
	lastTLS         bool
	generation      uint64
	refreshStop     chan struct{}
	refreshWG       sync.WaitGroup
	refreshInterval time.Duration
	newAdvertiser   func() discovery.Advertiser
	buildSpec       func(*config.Config, int, bool) (discovery.ServiceSpec, error)
}

func newZeroconfAdvertiser() discovery.Advertiser {
	return discovery.NewZeroconfAdvertiser()
}

func newDiscoveryAdvertiserManager() *discoveryAdvertiserManager {
	return &discoveryAdvertiserManager{
		newAdvertiser: newZeroconfAdvertiser,
		buildSpec:     discovery.BuildServiceSpec,
	}
}

func (s *Service) getDiscoveryManager() *discoveryAdvertiserManager {
	if s == nil {
		return nil
	}
	s.cfgMu.Lock()
	defer s.cfgMu.Unlock()
	if s.discoveryManager == nil {
		s.discoveryManager = newDiscoveryAdvertiserManager()
	}
	return s.discoveryManager
}

func (s *Service) applyDiscoveryConfig(cfg *config.Config) {
	s.applyDiscoveryConfigContext(context.Background(), cfg)
}

func (s *Service) applyDiscoveryConfigContext(ctx context.Context, cfg *config.Config) bool {
	if s == nil || cfg == nil || (ctx != nil && ctx.Err() != nil) {
		return false
	}
	mgr := s.getDiscoveryManager()
	if mgr == nil {
		return false
	}
	return mgr.ApplyContext(ctx, cfg, cfg.Port, cfg.TLS.Enable)
}

func (s *Service) shutdownDiscovery() error {
	if s == nil {
		return nil
	}
	mgr := s.getDiscoveryManager()
	if mgr == nil {
		return nil
	}
	return mgr.Shutdown()
}

func (m *discoveryAdvertiserManager) ApplyContext(ctx context.Context, cfg *config.Config, port int, tlsEnabled bool) bool {
	if m == nil || cfg == nil || (ctx != nil && ctx.Err() != nil) {
		return false
	}

	m.mu.Lock()
	m.lastCfg = cfg
	m.lastPort = port
	m.lastTLS = tlsEnabled

	if !cfg.Discovery.Enabled {
		oldAdv := m.advertiser
		refreshStop := m.detachRefreshLocked()
		m.advertiser = nil
		m.enabled = false
		m.lastSpec = discovery.ServiceSpec{}
		m.generation++
		m.mu.Unlock()

		m.stopRefresh(refreshStop)
		if oldAdv != nil {
			log.Info("discovery: stopping mDNS advertisement (disabled by config)")
			_ = oldAdv.Stop()
		}
		return true
	}

	m.ensureRefreshLocked()
	buildSpec := m.buildSpec
	newAdvertiser := m.newAdvertiser
	m.mu.Unlock()

	if buildSpec == nil {
		buildSpec = discovery.BuildServiceSpec
	}
	spec, err := buildSpec(cfg, port, tlsEnabled)
	if err != nil {
		log.Warnf("discovery: failed to build service spec: %v", err)
		return false
	}

	m.mu.Lock()
	if m.lastCfg != cfg || !cfg.Discovery.Enabled {
		m.mu.Unlock()
		return false
	}
	if m.enabled && m.advertiser != nil && specEqual(m.lastSpec, spec) {
		m.mu.Unlock()
		return true
	}

	oldAdv := m.advertiser
	m.advertiser = nil
	m.generation++
	gen := m.generation
	m.mu.Unlock()

	if oldAdv != nil {
		_ = oldAdv.Stop()
	}
	if newAdvertiser == nil {
		newAdvertiser = newZeroconfAdvertiser
	}
	adv := newAdvertiser()
	errStart := adv.Start(ctx, spec)

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.generation != gen {
		if adv != nil {
			m.mu.Unlock()
			_ = adv.Stop()
			m.mu.Lock()
		}
		return false
	}

	if errStart != nil {
		m.enabled = false
		log.Warnf("discovery: failed to start mDNS advertiser: %v (degraded, HTTP intact)", errStart)
		return false
	}

	m.advertiser = adv
	m.enabled = true
	m.lastSpec = spec
	log.Infof("discovery: advertising as '%s.%s' on port %d", spec.InstanceName, spec.ServiceType, port)
	return true
}

func (m *discoveryAdvertiserManager) Shutdown() error {
	m.mu.Lock()
	oldAdv := m.advertiser
	refreshStop := m.detachRefreshLocked()
	m.advertiser = nil
	m.enabled = false
	m.lastSpec = discovery.ServiceSpec{}
	m.lastCfg = nil
	m.generation++
	m.mu.Unlock()

	m.stopRefresh(refreshStop)
	if oldAdv != nil {
		return oldAdv.Stop()
	}
	return nil
}

func (m *discoveryAdvertiserManager) refreshPeriod() (time.Duration, bool) {
	if m.refreshInterval < 0 {
		return 0, false
	}
	if m.refreshInterval == 0 {
		return defaultDiscoveryRefreshInterval, true
	}
	return m.refreshInterval, true
}

func (m *discoveryAdvertiserManager) ensureRefreshLocked() {
	if _, ok := m.refreshPeriod(); !ok {
		return
	}
	if m.refreshStop != nil {
		return
	}
	stop := make(chan struct{})
	m.refreshStop = stop
	interval, _ := m.refreshPeriod()
	m.refreshWG.Add(1)
	go func() {
		defer m.refreshWG.Done()
		m.refreshLoop(stop, interval)
	}()
}

func (m *discoveryAdvertiserManager) detachRefreshLocked() chan struct{} {
	stop := m.refreshStop
	m.refreshStop = nil
	return stop
}

func (m *discoveryAdvertiserManager) stopRefresh(stop chan struct{}) {
	if stop == nil {
		return
	}
	close(stop)
	m.refreshWG.Wait()
}

func (m *discoveryAdvertiserManager) refreshLoop(stop <-chan struct{}, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			m.mu.Lock()
			cfg := m.lastCfg
			port := m.lastPort
			tlsEnabled := m.lastTLS
			m.mu.Unlock()
			if cfg == nil || !cfg.Discovery.Enabled {
				continue
			}
			_ = m.ApplyContext(context.Background(), cfg, port, tlsEnabled)
		}
	}
}

func specEqual(a, b discovery.ServiceSpec) bool {
	if a.InstanceName != b.InstanceName ||
		a.ServiceType != b.ServiceType ||
		a.Domain != b.Domain ||
		a.Port != b.Port ||
		len(a.Subtypes) != len(b.Subtypes) ||
		len(a.TextRecords) != len(b.TextRecords) ||
		len(a.Interfaces) != len(b.Interfaces) ||
		len(a.AdvertisedIPs) != len(b.AdvertisedIPs) {
		return false
	}
	for i := range a.Subtypes {
		if a.Subtypes[i] != b.Subtypes[i] {
			return false
		}
	}
	for i := range a.TextRecords {
		if a.TextRecords[i] != b.TextRecords[i] {
			return false
		}
	}
	for i := range a.Interfaces {
		if a.Interfaces[i].Name != b.Interfaces[i].Name {
			return false
		}
	}
	for i := range a.AdvertisedIPs {
		if a.AdvertisedIPs[i] != b.AdvertisedIPs[i] {
			return false
		}
	}
	return true
}
