package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/discovery"
)

func newLANBrowser() (discovery.Browser, error) {
	ifaces, err := discovery.FilterInterfaces(nil, nil)
	if err != nil {
		return nil, err
	}
	if len(ifaces) == 0 {
		return nil, fmt.Errorf("no qualified physical interfaces found for LAN discovery")
	}
	return discovery.NewZeroconfBrowser(ifaces...), nil
}

// DoDiscover executes the LAN AI gateway discovery workflow and outputs results.
// Returns 0 on success, 1 on error.
func DoDiscover(timeout time.Duration, jsonOutput bool) int {
	return runDiscover(timeout, jsonOutput, os.Stdout, os.Stderr, newLANBrowser)
}

func runDiscover(timeout time.Duration, jsonOutput bool, stdout, stderr io.Writer, newBrowser func() (discovery.Browser, error)) int {
	if timeout <= 0 {
		timeout = 3 * time.Second
	} else if timeout > 60*time.Second {
		timeout = 60 * time.Second
	}

	if !jsonOutput {
		fmt.Fprintf(stdout, "Scanning LAN for AI Gateways (%s)... (timeout %v)\n", discovery.DefaultServiceType, timeout)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout+1*time.Second)
	defer cancel()

	browser, errBrowser := newBrowser()
	if errBrowser != nil {
		if jsonOutput {
			out, _ := json.MarshalIndent(map[string]any{"error": errBrowser.Error(), "gateways": []any{}}, "", "  ")
			fmt.Fprintln(stdout, string(out))
		} else {
			fmt.Fprintf(stderr, "Error scanning LAN: %v\n", errBrowser)
		}
		return 1
	}
	gateways, err := browser.BrowseWithFallback(ctx, timeout)
	if err != nil {
		if jsonOutput {
			out, _ := json.MarshalIndent(map[string]any{"error": err.Error(), "gateways": []any{}}, "", "  ")
			fmt.Fprintln(stdout, string(out))
		} else {
			fmt.Fprintf(stderr, "Error scanning LAN: %v\n", err)
		}
		return 1
	}

	if gateways == nil {
		gateways = []discovery.DiscoveredService{}
	}

	if jsonOutput {
		out, errMarshal := json.MarshalIndent(gateways, "", "  ")
		if errMarshal != nil {
			fmt.Fprintf(stderr, "failed to marshal results: %v\n", errMarshal)
			return 1
		}
		fmt.Fprintln(stdout, string(out))
		return 0
	}

	if len(gateways) == 0 {
		fmt.Fprintln(stdout, "\nNo AI gateways found on local network.")
		fmt.Fprintln(stdout, "Tips:")
		fmt.Fprintln(stdout, "  1. Ensure the target CPA instance has 'discovery.enabled: true' in its config.yaml.")
		fmt.Fprintln(stdout, "  2. Ensure your device is on the same local Wi-Fi / Ethernet subnet (mDNS does not traverse WAN).")
		fmt.Fprintln(stdout, "  3. Check that your local firewall allows UDP port 5353 multicast traffic.")
		return 0
	}

	fmt.Fprintf(stdout, "\nFound %d AI Gateway(s) on local network:\n\n", len(gateways))
	for i, gw := range gateways {
		productLabel := sanitizeTerminal(gw.Product)
		if productLabel == "" {
			productLabel = "generic"
		}
		versionLabel := sanitizeTerminal(gw.Version)
		if versionLabel == "" {
			versionLabel = "unknown"
		}

		instanceLabel := sanitizeTerminal(gw.InstanceName)
		fmt.Fprintf(stdout, "[%d] %s (Product: %s, Version: %s)\n", i+1, instanceLabel, productLabel, versionLabel)

		// Primary IP selection
		primaryIP := "127.0.0.1"
		var allIPs []string
		for _, ip := range gw.IPv4 {
			if !ip.IsLoopback() {
				allIPs = append(allIPs, ip.String())
			}
		}
		if len(allIPs) > 0 {
			primaryIP = allIPs[0]
		} else if len(gw.IPv6) > 0 {
			primaryIP = gw.IPv6[0].String()
			for _, ip := range gw.IPv6 {
				allIPs = append(allIPs, ip.String())
			}
		}

		hostPort := net.JoinHostPort(primaryIP, strconv.Itoa(gw.Port))
		hostLabel := sanitizeTerminal(gw.Host)
		fmt.Fprintf(stdout, "    Host:      %s (%s)\n", hostLabel, hostPort)
		if len(allIPs) > 1 {
			fmt.Fprintf(stdout, "    Addresses: %s\n", strings.Join(allIPs, ", "))
		}

		if len(gw.Protocols) > 0 {
			var safeProtocols []string
			for _, p := range gw.Protocols {
				safeProtocols = append(safeProtocols, sanitizeTerminal(p))
			}
			fmt.Fprintf(stdout, "    Protocols: %s\n", strings.Join(safeProtocols, ", "))
		}
		if len(gw.Features) > 0 {
			var safeFeatures []string
			for _, f := range gw.Features {
				safeFeatures = append(safeFeatures, sanitizeTerminal(f))
			}
			fmt.Fprintf(stdout, "    Features:  %s\n", strings.Join(safeFeatures, ", "))
		}

		authStatus := "No"
		if gw.AuthRequired {
			authStatus = "Required"
			if len(gw.AuthMethods) > 0 {
				var safeMethods []string
				for _, m := range gw.AuthMethods {
					safeMethods = append(safeMethods, sanitizeTerminal(m))
				}
				authStatus = fmt.Sprintf("Required (%s)", strings.Join(safeMethods, ", "))
			}
		}
		fmt.Fprintf(stdout, "    Auth:      %s\n", authStatus)

		// Endpoints display
		scheme := "http"
		if gw.RawTXT["tls"] == "1" {
			scheme = "https"
		}

		fmt.Fprintf(stdout, "    Base URLs:\n")
		if openAIPath, ok := gw.Endpoints["openai"]; ok {
			fmt.Fprintf(stdout, "      - OpenAI:    %s://%s%s\n", scheme, hostPort, sanitizeTerminal(openAIPath))
		} else {
			fmt.Fprintf(stdout, "      - OpenAI:    %s://%s/v1\n", scheme, hostPort)
		}

		if anthropicPath, ok := gw.Endpoints["anthropic"]; ok {
			fmt.Fprintf(stdout, "      - Anthropic: %s://%s%s\n", scheme, hostPort, sanitizeTerminal(anthropicPath))
		}
		if geminiPath, ok := gw.Endpoints["gemini"]; ok {
			fmt.Fprintf(stdout, "      - Gemini:    %s://%s%s\n", scheme, hostPort, sanitizeTerminal(geminiPath))
		}

		fmt.Fprintln(stdout)
	}

	return 0
}

// sanitizeTerminal strips control characters, ANSI escape sequences, Bidi overrides,
// and invisible Unicode format characters to prevent terminal injection and spoofing.
func sanitizeTerminal(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == '\t' {
			b.WriteRune(' ')
			continue
		}
		// Allow safe printable runes; block Bidi controls, zero-width chars, and line/paragraph separators
		if unicode.IsPrint(r) && !unicode.Is(unicode.Bidi_Control, r) {
			if r != '\u2028' && r != '\u2029' && !(r >= 0x200B && r <= 0x200F) && r != '\uFEFF' && r != 0x00AD {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}
