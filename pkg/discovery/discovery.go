package discovery

import (
	"context"
	"fmt"
	"os"
	"strings"
	"tarun-kavipurapu/p2p-transfer/pkg/logger"

	"github.com/grandcat/zeroconf"
)

const (
	// ServiceType defines the mDNS service type for p2p-transfer
	ServiceType = "_p2p-transfer._tcp"
	// Domain is the local domain for mDNS
	Domain = "local."
)

// ServiceInfo contains information about a discovered service
type ServiceInfo struct {
	InstanceName string
	HostName     string
	Port         int
	IPs          []string
	Meta         map[string]string
}

// Advertiser handles service broadcasting
type Advertiser struct {
	server *zeroconf.Server
}

// Resolver handles service discovery
type Resolver struct {
	resolver *zeroconf.Resolver
}

// NewAdvertiser creates a new service advertiser
func NewAdvertiser() *Advertiser {
	return &Advertiser{}
}

// Start begins broadcasting the service
func (a *Advertiser) Start(instanceName string, port int, meta map[string]string) error {
	// If no instance name provided, use hostname
	if instanceName == "" {
		hostname, err := os.Hostname()
		if err != nil {
			instanceName = "p2p-server"
		} else {
			instanceName = fmt.Sprintf("p2p-server-%s", hostname)
		}
	}

	// Text records for metadata
	var txtRecords []string
	if meta != nil {
		for k, v := range meta {
			txtRecords = append(txtRecords, fmt.Sprintf("%s=%s", k, v))
		}
	}

	// Register the service
	// instanceName: Name of the service instance (e.g. "My P2P Server")
	// ServiceType:  Service type (e.g. "_p2p-transfer._tcp")
	// Domain:       Domain (e.g. "local.")
	// Port:         Port where the service is running
	// Text:         TXT records
	// Ifaces:       Interfaces to bind (nil = all)
	server, err := zeroconf.Register(
		instanceName,
		ServiceType,
		Domain,
		port,
		txtRecords,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to register mDNS service: %w", err)
	}

	a.server = server
	return nil
}

// Stop stops broadcasting the service
func (a *Advertiser) Stop() {
	if a.server != nil {
		a.server.Shutdown()
		a.server = nil
	}
}

// NewResolver creates a new service resolver
func NewResolver() (*Resolver, error) {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create mDNS resolver: %w", err)
	}
	return &Resolver{resolver: resolver}, nil
}

// Browse scans for services until the context is canceled
// It returns a channel that will receive discovered services
func (r *Resolver) Browse(ctx context.Context) (<-chan *ServiceInfo, error) {
	entries := make(chan *zeroconf.ServiceEntry)
	// 设置缓冲区大小
	results := make(chan *ServiceInfo, 10)

	// Start browsing in background
	if err := r.resolver.Browse(ctx, ServiceType, Domain, entries); err != nil {
		return nil, fmt.Errorf("failed to browse services: %w", err)
	}

	// Process results
	go func() {
		defer close(results)

		for {
			select {
			case <-ctx.Done():
				return
			case entry, ok := <-entries:
				if !ok {
					return
				}

				// Convert zeroconf entry to our ServiceInfo
				info := &ServiceInfo{
					InstanceName: entry.Instance,
					HostName:     entry.HostName,
					Port:         entry.Port,
					IPs:          make([]string, 0),
					Meta:         make(map[string]string),
				}

				// Filter IPv4
				for _, ip := range entry.AddrIPv4 {
					info.IPs = append(info.IPs, ip.String())
				}

				// Parse TXT records
				for _, record := range entry.Text {
					parts := strings.SplitN(record, "=", 2)
					if len(parts) == 2 {
						info.Meta[parts[0]] = parts[1]
					}
				}

				// Only send if we found valid IPs
				if len(info.IPs) > 0 {
					logger.Sugar.Infof("[Discovery] discovered service: instance=%s ips=%v port=%d", info.InstanceName, info.IPs, info.Port)
					results <- info
				}
			}
		}
	}()

	return results, nil
}
