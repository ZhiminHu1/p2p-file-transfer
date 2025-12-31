package discovery

import (
	"context"
	"testing"
	"time"
)

func TestDiscovery(t *testing.T) {
	// Skip in CI/docker environments where multicast might not work
	if testing.Short() {
		t.Skip("Skipping mDNS test in short mode")
	}

	// 1. Start Advertiser
	advertiser := NewAdvertiser()
	meta := map[string]string{"test": "true"}
	port := 12345

	err := advertiser.Start("test-service", port, meta)
	if err != nil {
		t.Fatalf("Failed to start advertiser: %v", err)
	}
	defer advertiser.Stop()

	// Give it a moment to announce
	time.Sleep(500 * time.Millisecond)

	// 2. Start Resolver
	resolver, err := NewResolver()
	if err != nil {
		t.Fatalf("Failed to create resolver: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, err := resolver.Browse(ctx)
	if err != nil {
		t.Fatalf("Failed to browse: %v", err)
	}

	// 3. Verify Discovery
	found := false
	for info := range ch {
		if info.Port == port && info.Meta["test"] == "true" {
			found = true
			if len(info.IPs) == 0 {
				t.Error("Discovered service has no IPs")
			}
			t.Logf("Found service: %+v", info)
			break
		}
	}

	if !found {
		t.Error("Failed to discover the test service")
	}
}
