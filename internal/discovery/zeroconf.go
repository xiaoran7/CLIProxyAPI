package discovery

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/libp2p/zeroconf/v2"
	log "github.com/sirupsen/logrus"
)

// ZeroconfAdvertiser wraps libp2p/zeroconf/v2 Server with defensive lifecycle management.
type ZeroconfAdvertiser struct {
	mu       sync.Mutex
	server   *zeroconf.Server
	inFlight bool
	closed   bool
}

// NewZeroconfAdvertiser returns a new ZeroconfAdvertiser.
func NewZeroconfAdvertiser() *ZeroconfAdvertiser {
	return &ZeroconfAdvertiser{}
}

// extractCleanHost returns bare hostname without any .local suffix
// to prevent libp2p/zeroconf/v2 from appending a duplicate .local. (e.g. on macOS).
func extractCleanHost() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "localhost"
	}
	h = strings.TrimSpace(h)
	h = strings.TrimSuffix(h, ".")
	h = strings.TrimSuffix(h, ".local")
	h = strings.TrimSuffix(h, ".")
	if h == "" {
		return "localhost"
	}
	return h
}

// extractInterfaceIPs collects non-loopback IP addresses from the selected interfaces.
func extractInterfaceIPs(ifaces []net.Interface) []string {
	var ips []string
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() {
				continue
			}
			ips = append(ips, ip.String())
		}
	}
	return ips
}

const instanceNameProbeTimeout = 400 * time.Millisecond

// Start registers and starts mDNS advertisement for the primary service type
// and any configured API protocol subtypes.
func (a *ZeroconfAdvertiser) Start(ctx context.Context, spec ServiceSpec) (err error) {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if len(spec.Interfaces) == 0 {
		return fmt.Errorf("discovery: cannot start advertiser with empty interface list (refusing fallback to all interfaces)")
	}
	if spec.Port < 1 || spec.Port > 65535 {
		return fmt.Errorf("discovery: invalid service port %d (must be between 1 and 65535)", spec.Port)
	}

	a.mu.Lock()
	if a.server != nil || a.inFlight {
		a.mu.Unlock()
		return fmt.Errorf("discovery: advertiser already started")
	}
	a.inFlight = true
	a.closed = false
	a.mu.Unlock()

	var server *zeroconf.Server
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("discovery: panic starting advertiser: %v", r)
			log.Errorf("%v", err)
		}
		a.mu.Lock()
		a.inFlight = false
		if err != nil {
			hold := a.server
			a.server = nil
			a.mu.Unlock()
			if hold != nil {
				hold.Shutdown()
			}
			if server != nil && hold != server {
				server.Shutdown()
			}
			return
		}
		a.mu.Unlock()
	}()

	domain := spec.Domain
	if domain == "" {
		domain = DefaultDomain
	}
	serviceType := spec.ServiceType
	if serviceType == "" {
		serviceType = DefaultServiceType
	}

	cleanHost := extractCleanHost()
	ips := spec.AdvertisedIPs
	if len(ips) == 0 {
		ips = extractInterfaceIPs(spec.Interfaces)
	}
	if len(ips) == 0 {
		return fmt.Errorf("discovery: no usable IP addresses found on specified interfaces")
	}

	spec.InstanceName = uniquifyInstanceName(spec.InstanceName, browseTakenNames(ctx, spec, ips))

	primaryService := serviceType
	for _, sub := range spec.Subtypes {
		if clean := sanitizeSubtype(sub); clean != "" {
			primaryService += "," + clean
		}
	}

	var errRegister error
	server, errRegister = zeroconf.RegisterProxy(
		spec.InstanceName,
		primaryService,
		domain,
		spec.Port,
		cleanHost,
		ips,
		spec.TextRecords,
		spec.Interfaces,
	)
	if errRegister != nil {
		return fmt.Errorf("discovery: failed to register primary service %s: %w", primaryService, errRegister)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return fmt.Errorf("discovery: advertiser stopped before start completed")
	}
	a.server = server
	server = nil
	return nil
}

func browseTakenNames(ctx context.Context, spec ServiceSpec, ips []string) map[string]struct{} {
	timeout := instanceNameProbeTimeout
	if ctx != nil {
		if deadline, ok := ctx.Deadline(); ok {
			remaining := time.Until(deadline)
			if remaining <= 0 {
				return nil
			}
			if remaining < timeout {
				timeout = remaining
			}
		}
	}
	browser := NewZeroconfBrowser(spec.Interfaces...)
	found, err := browser.Browse(ctx, spec.ServiceType, spec.Domain, timeout)
	if err != nil {
		log.Debugf("discovery: name uniqueness browse failed: %v", err)
		return nil
	}
	return takenInstanceNames(found, spec.Port, ips)
}

// Stop shuts down the mDNS advertisement server and sends goodbye packets.
func (a *ZeroconfAdvertiser) Stop() error {
	a.mu.Lock()
	a.closed = true
	server := a.server
	a.server = nil
	a.mu.Unlock()
	if server == nil {
		return nil
	}
	defer func() {
		if r := recover(); r != nil {
			log.Warnf("discovery: panic during advertiser shutdown: %v", r)
		}
	}()
	server.Shutdown()
	return nil
}

// ZeroconfBrowser provides DNS-SD browsing with fallback support.
type ZeroconfBrowser struct {
	options []zeroconf.ClientOption
}

// NewZeroconfBrowser creates a new ZeroconfBrowser.
func NewZeroconfBrowser(ifaces ...net.Interface) *ZeroconfBrowser {
	var opts []zeroconf.ClientOption
	if len(ifaces) > 0 {
		opts = append(opts, zeroconf.SelectIfaces(ifaces))
	}
	return &ZeroconfBrowser{options: opts}
}

const (
	maxDiscoveredServices = 256
	maxBrowseTXTRecords   = 64
	maxBrowseTXTBytes     = 16 * 1024
)

func browseEntryWithinLimits(entry *zeroconf.ServiceEntry) bool {
	if entry == nil || len(entry.Text) > maxBrowseTXTRecords {
		return false
	}
	total := 0
	for _, record := range entry.Text {
		if len(record) > maxTXTRecordBytes {
			return false
		}
		total += len(record) + 1
		if total > maxBrowseTXTBytes {
			return false
		}
	}
	return true
}

// Browse performs a standard mDNS browse query for the given service type.
func (b *ZeroconfBrowser) Browse(ctx context.Context, serviceType, domain string, timeout time.Duration) ([]DiscoveredService, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if domain == "" {
		domain = DefaultDomain
	}
	if serviceType == "" {
		serviceType = DefaultServiceType
	}

	ctxTimeout, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// If parent context is canceled earlier, cancel timeout context immediately
	go func() {
		select {
		case <-ctxTimeout.Done():
		case <-ctx.Done():
			cancel()
		}
	}()

	entries := make(chan *zeroconf.ServiceEntry, 32)
	var discovered []DiscoveredService
	var mu sync.Mutex
	seen := make(map[string]bool)

	// Collect entries in background until channel is closed by zeroconf's params.done()
	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)
		accepted := 0
		for entry := range entries {
			// Keep draining the channel so zeroconf can shut down cleanly, but
			// stop parsing attacker-controlled entries after the result cap.
			if accepted >= maxDiscoveredServices {
				continue
			}
			if !browseEntryWithinLimits(entry) {
				continue
			}
			svc := entryToDiscovered(entry)
			if svc.Port == 0 || (len(svc.IPv4) == 0 && len(svc.IPv6) == 0) {
				continue
			}
			key := fmt.Sprintf("%s:%s:%d", svc.InstanceName, svc.Host, svc.Port)
			mu.Lock()
			if _, ok := seen[key]; ok || len(seen) >= maxDiscoveredServices {
				mu.Unlock()
				continue
			}
			seen[key] = true
			discovered = append(discovered, svc)
			accepted++
			mu.Unlock()
		}
	}()

	errBrowse := zeroconf.Browse(ctxTimeout, serviceType, domain, entries, b.options...)
	if errBrowse != nil {
		cancel()
		// zeroconf failed before launching mainloop; close entries so collector terminates cleanly
		close(entries)
		<-doneCh
		return nil, fmt.Errorf("discovery: browse query failed: %w", errBrowse)
	}

	<-doneCh

	return discovered, nil
}

// BrowseWithFallback discovers all AI gateways on the LAN (_ai-gateway._tcp) and prioritizes CPA instances.
func (b *ZeroconfBrowser) BrowseWithFallback(ctx context.Context, timeout time.Duration) ([]DiscoveredService, error) {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}

	allGateways, errMain := b.Browse(ctx, DefaultServiceType, DefaultDomain, timeout)
	if errMain != nil && len(allGateways) == 0 {
		return nil, errMain
	}

	// Partition: prioritize CPA instances first, then other standard AI gateways
	var cpaGateways []DiscoveredService
	var otherGateways []DiscoveredService
	for _, gw := range allGateways {
		if gw.Product == ProductCPA {
			cpaGateways = append(cpaGateways, gw)
		} else {
			otherGateways = append(otherGateways, gw)
		}
	}

	return append(cpaGateways, otherGateways...), nil
}

// sanitizeEndpointPath validates that an endpoint path is a safe relative API path
// starting with '/' and containing no scheme, domain, protocol-relative prefixes, or traversal elements.
func sanitizeEndpointPath(p string) string {
	p = strings.TrimSpace(p)
	if !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") || strings.Contains(p, `\`) || strings.Contains(p, "://") || strings.Contains(p, "..") {
		return ""
	}
	for _, r := range p {
		if r < 32 || r == 127 {
			return ""
		}
	}
	return p
}

func entryToDiscovered(e *zeroconf.ServiceEntry) DiscoveredService {
	parsed := ParseTXTRecords(e.Text)

	port := e.Port
	if port < 1 || port > 65535 {
		port = 0
	}

	svc := DiscoveredService{
		InstanceName: e.Instance,
		ServiceType:  e.Service,
		Domain:       e.Domain,
		Host:         e.HostName,
		Port:         port,
		IPv4:         filterUsableIPs(e.AddrIPv4),
		IPv6:         filterUsableIPs(e.AddrIPv6),
		Product:      parsed["product"],
		Version:      parsed["version"],
		NodeRole:     parsed["node_role"],
		RawTXT:       parsed,
		Endpoints:    make(map[string]string),
	}

	if parsed["auth_required"] == "true" {
		svc.AuthRequired = true
	}
	if methods := parsed["auth_methods"]; methods != "" {
		svc.AuthMethods = strings.Split(methods, ",")
	}
	if protos := parsed["protocols"]; protos != "" {
		svc.Protocols = strings.Split(protos, ",")
	}
	if feats := parsed["features"]; feats != "" {
		svc.Features = strings.Split(feats, ",")
	}

	if v, ok := parsed["api_openai"]; ok {
		if clean := sanitizeEndpointPath(v); clean != "" {
			svc.Endpoints["openai"] = clean
		}
	}
	if v, ok := parsed["api_anthropic"]; ok {
		if clean := sanitizeEndpointPath(v); clean != "" {
			svc.Endpoints["anthropic"] = clean
		}
	}
	if v, ok := parsed["api_gemini"]; ok {
		if clean := sanitizeEndpointPath(v); clean != "" {
			svc.Endpoints["gemini"] = clean
		}
	}

	return svc
}

func filterUsableIPs(ips []net.IP) []net.IP {
	usable := make([]net.IP, 0, len(ips))
	for _, ip := range ips {
		if ip == nil || ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() {
			continue
		}
		usable = append(usable, ip)
	}
	return usable
}
