package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/discovery"
)

type fakeBrowser struct {
	result      []discovery.DiscoveredService
	err         error
	serviceType string
}

func (f *fakeBrowser) Browse(context.Context, string, string) ([]discovery.DiscoveredService, error) {
	return f.result, f.err
}

func (f *fakeBrowser) BrowseWithFallback(context.Context) ([]discovery.DiscoveredService, error) {
	return f.result, f.err
}

func (f *fakeBrowser) BrowseWithFallbackServiceType(_ context.Context, serviceType string) ([]discovery.DiscoveredService, error) {
	f.serviceType = serviceType
	return f.result, f.err
}

func TestRunDiscoverJSONEmpty(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runDiscover(0, true, &stdout, &stderr, func() (discovery.Browser, error) {
		return &fakeBrowser{result: nil}, nil
	})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	var got []discovery.DiscoveredService
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json: %v\nstdout=%s", err, stdout.String())
	}
	if len(got) != 0 {
		t.Fatalf("gateways = %#v, want empty array", got)
	}
}

func TestRunDiscoverCustomServiceType(t *testing.T) {
	var stdout, stderr bytes.Buffer
	browser := &fakeBrowser{}
	code := runDiscoverWithServiceType(2*time.Second, true, "_custom._tcp", &stdout, &stderr, func() (discovery.Browser, error) {
		return browser, nil
	})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if browser.serviceType != "_custom._tcp" {
		t.Fatalf("service type = %q, want _custom._tcp", browser.serviceType)
	}
}

func TestRunDiscoverJSONError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runDiscover(2*time.Second, true, &stdout, &stderr, func() (discovery.Browser, error) {
		return nil, errNoIfaces
	})
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("json: %v\nstdout=%s", err, stdout.String())
	}
	if payload["error"] == nil {
		t.Fatalf("expected error field, got %#v", payload)
	}
}

func TestRunDiscoverClampsTimeoutAndPrintsText(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runDiscover(2*time.Minute, false, &stdout, &stderr, func() (discovery.Browser, error) {
		return &fakeBrowser{result: []discovery.DiscoveredService{{
			InstanceName: "office-gateway",
			Product:      "cliproxyapi",
			Version:      "1",
			Port:         8317,
			IPv4:         []net.IP{net.ParseIP("192.0.2.10")},
			Endpoints:    map[string]string{"openai": "/v1"},
			RawTXT:       map[string]string{"tls": "0"},
		}}}, nil
	})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "timeout 1m0s") {
		t.Fatalf("expected timeout clamped to 60s, got %q", out)
	}
	if !strings.Contains(out, "office-gateway") {
		t.Fatalf("expected instance name in output, got %q", out)
	}
}

type staticError string

func (e staticError) Error() string { return string(e) }

const errNoIfaces staticError = "no qualified physical interfaces found for LAN discovery"
