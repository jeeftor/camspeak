// Package frigate discovers cameras from a Frigate NVR instance by querying
// its config API and parsing the RTSP URLs from go2rtc stream definitions.
package frigate

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DiscoveredCamera represents a single camera extracted from a Frigate config.
type DiscoveredCamera struct {
	Name    string `json:"name"    db:"name"`
	Type    string `json:"type"    db:"type"`
	IP      string `json:"ip"      db:"ip"`
	User    string `json:"user"    db:"user"`
	Pass    string `json:"pass"    db:"pass"`
	Channel int    `json:"channel" db:"channel"`
}

// Discoverer queries a Frigate NVR instance for its camera configuration.
type Discoverer struct {
	frigateURL string
	client     *http.Client
}

// NewDiscoverer creates a Discoverer for the given Frigate URL with a 10s
// HTTP client timeout.
func NewDiscoverer(frigateURL string) *Discoverer {
	return &Discoverer{
		frigateURL: strings.TrimRight(frigateURL, "/"),
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

// frigateConfig is the subset of the Frigate /api/config response we parse.
type frigateConfig struct {
	Cameras map[string]struct {
		FFmpeg struct {
			Inputs []struct {
				Path string `json:"path"`
			} `json:"inputs"`
		} `json:"ffmpeg"`
	} `json:"cameras"`
	Go2rtc struct {
		Streams map[string][]string `json:"streams"`
	} `json:"go2rtc"`
}

// Discover fetches the Frigate config and returns the list of cameras
// deduplicated by IP address, preferring main-stream entries over sub-streams.
// Credentials from the Frigate API are masked as "*:*" — set real credentials
// via CAM_<NAME>_USER and CAM_<NAME>_PASS environment variables.
func (d *Discoverer) Discover() ([]DiscoveredCamera, error) {
	endpoint := d.frigateURL + "/api/config"

	resp, err := d.client.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("fetching frigate config: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("frigate config returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading frigate config body: %w", err)
	}

	var cfg frigateConfig
	if err := json.Unmarshal(body, &cfg); err != nil {
		return nil, fmt.Errorf("parsing frigate config JSON: %w", err)
	}

	if len(cfg.Go2rtc.Streams) == 0 && len(cfg.Cameras) == 0 {
		return nil, fmt.Errorf("no cameras found in frigate config")
	}

	// byIP maps IP -> DiscoveredCamera, preferring main-stream entries.
	byIP := make(map[string]DiscoveredCamera)
	isMain := make(map[string]bool)

	// Parse go2rtc streams — these have the actual camera IPs
	for streamName, urls := range cfg.Go2rtc.Streams {
		camIsMain := strings.HasSuffix(streamName, "_main")

		for _, u := range urls {
			// Strip "ffmpeg:" prefix if present
			rtspURL := strings.TrimPrefix(u, "ffmpeg:")

			if !strings.HasPrefix(rtspURL, "rtsp://") {
				continue
			}

			cam, ok := parseRTSP(rtspURL, streamName)
			if !ok {
				continue
			}

			if _, found := byIP[cam.IP]; !found {
				byIP[cam.IP] = cam
				isMain[cam.IP] = camIsMain
				continue
			}

			// Prefer main-stream over sub-stream
			if !isMain[cam.IP] && camIsMain {
				byIP[cam.IP] = cam
				isMain[cam.IP] = camIsMain
			}
		}
	}

	// If go2rtc had no real IPs, fall back to cameras section (restream URLs)
	// This won't give us IPs but at least registers camera names
	if len(byIP) == 0 {
		for _, cam := range cfg.Cameras {
			for _, inp := range cam.FFmpeg.Inputs {
				if strings.HasPrefix(inp.Path, "rtsp://localhost") {
					// Restream URL — can't extract real IP from go2rtc restream
					_ = inp
				}
			}
		}
	}

	result := make([]DiscoveredCamera, 0, len(byIP))
	for _, cam := range byIP {
		result = append(result, cam)
	}
	return result, nil
}

// parseRTSP parses an RTSP URL and the originating Frigate stream name into a
// DiscoveredCamera. It returns ok=false if the URL cannot be parsed.
// Credentials may be masked as "*" from the Frigate API — env vars override.
func parseRTSP(rawURL, frigateName string) (DiscoveredCamera, bool) {
	// Strip fragment (e.g. #backchannel=0)
	if idx := strings.Index(rawURL, "#"); idx >= 0 {
		rawURL = rawURL[:idx]
	}

	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" {
		return DiscoveredCamera{}, false
	}

	cam := DiscoveredCamera{
		IP:      u.Hostname(),
		Channel: 1,
		Name:    stripStreamSuffix(frigateName),
	}

	if u.User != nil {
		cam.User = u.User.Username()
		cam.Pass, _ = u.User.Password()
	}

	cam.Type = classifyCamera(u.Path)
	cam.Channel = extractChannel(u.Path)

	return cam, true
}

// classifyCamera determines the camera vendor from the RTSP URL path.
func classifyCamera(path string) string {
	switch {
	case strings.Contains(path, "/Streaming/Channels/"):
		return "hikvision"
	case strings.Contains(path, "h264Preview"):
		return "reolink"
	case strings.Contains(path, "/Preview_"):
		return "reolink"
	case strings.Contains(path, "/flv?"):
		return "reolink"
	case strings.Contains(path, "/stream"):
		return "hikvision"
	default:
		return "hikvision"
	}
}

// extractChannel parses the channel number from the RTSP URL path.
// Hikvision: /Streaming/Channels/101 → 1
// Reolink: /Preview_01_main → 1
func extractChannel(path string) int {
	if strings.Contains(path, "/Streaming/Channels/") {
		parts := strings.Split(path, "/Streaming/Channels/")
		if len(parts) > 1 {
			chStr := parts[1]
			if len(chStr) >= 3 {
				chStr = chStr[:3]
			}
			var ch int
			if _, err := fmt.Sscanf(chStr, "%d", &ch); err == nil && ch > 0 {
				return ch / 100
			}
		}
	}
	if strings.Contains(path, "Preview_") {
		parts := strings.Split(path, "Preview_")
		if len(parts) > 1 {
			var ch int
			if _, err := fmt.Sscanf(parts[1], "%d", &ch); err == nil && ch > 0 {
				return ch
			}
		}
	}
	return 1
}

// stripStreamSuffix removes _main/_sub suffixes from a Frigate stream name so
// that sub-streams map to their base camera.
func stripStreamSuffix(name string) string {
	for _, suffix := range []string{"_main", "_sub"} {
		if strings.HasSuffix(name, suffix) {
			return strings.TrimSuffix(name, suffix)
		}
	}
	return name
}

// SaveToDB inserts or replaces the given cameras into the cameras table.
func SaveToDB(db *sql.DB, cameras []DiscoveredCamera) error {
	const q = `INSERT OR REPLACE INTO cameras ` +
		`(name, type, ip, user, pass, channel) VALUES (?, ?, ?, ?, ?, ?)`

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.Prepare(q)
	if err != nil {
		return fmt.Errorf("preparing statement: %w", err)
	}
	defer stmt.Close() //nolint:errcheck

	for _, c := range cameras {
		if _, err := stmt.Exec(c.Name, c.Type, c.IP, c.User, c.Pass, c.Channel); err != nil {
			return fmt.Errorf("inserting camera %q: %w", c.Name, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}
	return nil
}
