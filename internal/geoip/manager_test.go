package geoip

import (
	"os"
	"sync"
	"testing"
	"time"
)

func TestManagerDegradedMode(t *testing.T) {
	// Initialize manager pointing to a non-existent path with auto-update disabled
	m := NewManager(Config{
		DBPath:     "/tmp/non_existent_geo_path_test.mmdb",
		AutoUpdate: false,
	})
	defer m.Close()

	// 1. Querying an internal IP should succeed instantly from memory
	internalLoc := m.Lookup("172.18.0.1")
	if internalLoc == nil || internalLoc.Country != "内网" || internalLoc.Region != "Docker" {
		t.Fatalf("unexpected internal IP location: %+v", internalLoc)
	}

	tailnetLoc := m.Lookup("100.72.34.104")
	if tailnetLoc == nil || tailnetLoc.Region != "Tailnet" {
		t.Fatalf("unexpected tailnet IP location: %+v", tailnetLoc)
	}

	// 2. Querying a public IP in degraded mode (no mmdb) must not panic and must return fallback
	pubLoc := m.Lookup("119.39.133.75")
	if pubLoc == nil || pubLoc.Country != "未知" {
		t.Fatalf("expected graceful fallback location in degraded mode, got %+v", pubLoc)
	}

	// 3. Querying empty or invalid IP
	if emptyLoc := m.Lookup(""); emptyLoc != nil {
		t.Fatalf("expected nil for empty IP, got %+v", emptyLoc)
	}
}

func TestManagerWithRealDatabase(t *testing.T) {
	const mmdbSample = "/tmp/Merged-IP.mmdb"
	if _, err := os.Stat(mmdbSample); os.IsNotExist(err) {
		t.Skip("skipping real database test: /tmp/Merged-IP.mmdb not present")
	}

	m := NewManager(Config{
		DBPath:     mmdbSample,
		AutoUpdate: false,
	})
	defer m.Close()

	// 1. Test domestic IP query (Hunan)
	hunanLoc := m.Lookup("119.39.133.75")
	if hunanLoc == nil {
		t.Fatal("expected location for 119.39.133.75, got nil")
	}
	if hunanLoc.Country != "中国" {
		t.Fatalf("expected Country='中国', got %q", hunanLoc.Country)
	}
	if hunanLoc.CountryCode != "CN" {
		t.Fatalf("expected CountryCode='CN', got %q", hunanLoc.CountryCode)
	}
	if hunanLoc.Region != "湖南" {
		t.Fatalf("expected Region='湖南', got %q", hunanLoc.Region)
	}
	if hunanLoc.City != "株洲" {
		t.Fatalf("expected City='株洲', got %q", hunanLoc.City)
	}
	if hunanLoc.ASN != 4837 {
		t.Fatalf("expected ASN=4837, got %d", hunanLoc.ASN)
	}

	// 2. Test international IP query (Google Anycast)
	usLoc := m.Lookup("8.8.8.8")
	if usLoc == nil || usLoc.CountryCode != "US" {
		t.Fatalf("expected US location for 8.8.8.8, got %+v", usLoc)
	}

	// 3. Test concurrent read safety
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = m.Lookup("119.39.133.75")
				_ = m.Lookup("172.18.0.1")
				_ = m.Lookup("8.8.8.8")
			}
		}()
	}
	wg.Wait()
}

func TestManagerConcurrentHotSwap(t *testing.T) {
	const mmdbSample = "/tmp/Merged-IP.mmdb"
	if _, err := os.Stat(mmdbSample); os.IsNotExist(err) {
		t.Skip("skipping hot-swap test: /tmp/Merged-IP.mmdb not present")
	}

	m := NewManager(Config{
		DBPath:     mmdbSample,
		AutoUpdate: false,
	})
	defer m.Close()

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// 20 concurrent readers
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					loc := m.Lookup("119.39.133.75")
					if loc == nil {
						t.Errorf("lookup returned nil during concurrent hot-swap")
						return
					}
				}
			}
		}()
	}

	// Simulate repeated hot-swap reloads
	for cycle := 0; cycle < 5; cycle++ {
		time.Sleep(20 * time.Millisecond)
		if err := m.loadReader(mmdbSample); err != nil {
			t.Fatalf("loadReader failed during cycle %d: %v", cycle, err)
		}
	}

	close(stop)
	wg.Wait()
}
