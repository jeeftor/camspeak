// Package discovery advertises camspeak via mDNS (Zeroconf/DNS-SD) so that
// Home Assistant and other clients can auto-discover the server on the local
// network.
//
// The service type is _camspeak._tcp.local. TXT records carry the port,
// version, and number of configured cameras so that HA's zeroconf scanner
// can pre-fill the config flow.
package discovery

import (
	"fmt"

	clog "github.com/charmbracelet/log"
	"github.com/grandcat/zeroconf"

	"github.com/jeeftor/camspeak/internal/logging"
)

var log = logging.New("discovery", clog.InfoLevel)

// SetLogLevel sets the log level for this package (called from cmd at startup).
func SetLogLevel(level clog.Level) {
	log = logging.New("discovery", level)
}

// Service advertises camspeak on the local network via mDNS.
type Service struct {
	server *zeroconf.Server
}

// Register advertises _camspeak._tcp.local with the given port and metadata.
// advertiseIP is optional (empty = auto-detect all interfaces); use it in
// Docker host-network mode where auto-detection picks the wrong interface.
func Register(port int, version string, cameraCount int, advertiseIP string) (*Service, error) {
	txt := []string{
		fmt.Sprintf("version=%s", version),
		fmt.Sprintf("cameras=%d", cameraCount),
		"protocol=https",
	}

	var server *zeroconf.Server
	var err error

	if advertiseIP != "" {
		server, err = zeroconf.RegisterProxy(
			"camspeak",
			"_camspeak._tcp",
			"local.",
			port,
			"camspeak",
			[]string{advertiseIP},
			txt,
			nil,
		)
	} else {
		server, err = zeroconf.Register(
			"camspeak",
			"_camspeak._tcp",
			"local.",
			port,
			txt,
			nil,
		)
	}

	if err != nil {
		return nil, fmt.Errorf("mDNS registration: %w", err)
	}

	log.Info("mDNS discovery advertised",
		"service", "_camspeak._tcp.local",
		"port", port,
		"version", version,
		"cameras", cameraCount,
		"advertise_ip", advertiseIP,
	)

	return &Service{server: server}, nil
}

// Shutdown stops the mDNS advertisement.
func (s *Service) Shutdown() {
	if s.server != nil {
		s.server.Shutdown()
	}
}
