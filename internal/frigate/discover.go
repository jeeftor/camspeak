// Package frigate discovers cameras from a Frigate NVR instance by querying
// its raw config endpoint and parsing the RTSP URLs of each configured camera.
package frigate

import (
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/viper"
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

// Discover fetches the Frigate raw config and returns the list of cameras
// deduplicated by IP address, preferring main-stream entries over sub-streams.
func (d *Discoverer) Discover() ([]DiscoveredCamera, error) {
	endpoint := d.frigateURL + "/config/raw"

	resp, err := d.client.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("fetching frigate config: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("frigate config returned status %d", resp.StatusCode)
	}

	yamlBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading frigate config body: %w", err)
	}

	v := viper.New()
	v.SetConfigType("yaml")
	if err := v.ReadConfig(bytes.NewReader(yamlBytes)); err != nil {
		return nil, fmt.Errorf("parsing frigate config yaml: %w", err)
	}

	cameras := v.GetStringMap("cameras")
	if len(cameras) == 0 {
		return nil, fmt.Errorf("no cameras found in frigate config")
	}

	// byIP maps IP -> DiscoveredCamera, preferring main-stream entries.
	// isMain tracks whether the stored entry originated from a "_main" stream.
	byIP := make(map[string]DiscoveredCamera)
	isMain := make(map[string]bool)

	for camName, camVal := range cameras {
		camMap, ok := camVal.(map[string]interface{})
		if !ok {
			continue
		}

		ffmpeg, ok := camMap["ffmpeg"].(map[string]interface{})
		if !ok {
			continue
		}
		inputs, ok := ffmpeg["inputs"].([]interface{})
		if !ok {
			continue
		}

		camIsMain := strings.HasSuffix(camName, "_main")

		for _, in := range inputs {
			inMap, ok := in.(map[string]interface{})
			if !ok {
				continue
			}

			path, _ := inMap["path"].(string)
			if !strings.HasPrefix(path, "rtsp://") {
				continue
			}

			cam, ok := parseRTSP(path, camName)
			if !ok {
				continue
			}

			if _, found := byIP[cam.IP]; !found {
				byIP[cam.IP] = cam
				isMain[cam.IP] = camIsMain
				continue
			}

			// Prefer the main-stream entry over a sub-stream entry.
			if !isMain[cam.IP] && camIsMain {
				byIP[cam.IP] = cam
				isMain[cam.IP] = camIsMain
			}
		}
	}

	result := make([]DiscoveredCamera, 0, len(byIP))
	for _, cam := range byIP {
		result = append(result, cam)
	}
	return result, nil
}

// parseRTSP parses an RTSP URL and the originating Frigate camera name into a
// DiscoveredCamera. It returns ok=false if the URL cannot be parsed.
func parseRTSP(rawURL, frigateName string) (DiscoveredCamera, bool) {
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

	return cam, true
}

// classifyCamera determines the camera vendor from the RTSP URL path.
func classifyCamera(path string) string {
	switch {
	case strings.Contains(path, "/Streaming/Channels/"):
		return "hikvision"
	case strings.Contains(path, "h264Preview"):
		return "reolink"
	case strings.Contains(path, "/flv?"):
		return "reolink"
	default:
		return "hikvision"
	}
}

// stripStreamSuffix removes _main/_sub suffixes from a Frigate camera name so
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
