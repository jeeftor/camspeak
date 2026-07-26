package cameras

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// ProbeCameraType attempts to identify the camera vendor by probing common APIs.
// It returns "hikvision", "reolink", "onvif", or "" if unknown.
func ProbeCameraType(ip, user, pass string) string {
	if ip == "" {
		return ""
	}

	// Try Reolink HTTP API first (fast, port 80, JSON).
	if t := probeReolink(ip, user, pass); t != "" {
		return t
	}

	// Try Hikvision ISAPI device info.
	if t := probeHikvision(ip, user, pass); t != "" {
		return t
	}

	// Last resort: try ONVIF GetDeviceInformation.
	if t := probeONVIF(ip, user, pass); t != "" {
		return t
	}

	return ""
}

// probeReolink queries the Reolink CGI GetDevInfo command.
func probeReolink(ip, user, pass string) string {
	apiURL := fmt.Sprintf("http://%s/cgi-bin/api.cgi?user=%s&password=%s", ip, user, pass)
	payload := `[{"cmd":"GetDevInfo","action":1,"param":{}}]`
	body, err := httpPostBody(apiURL, payload, 3*time.Second)
	if err != nil {
		return ""
	}
	if bytes.Contains(body, []byte(`"cmd":"GetDevInfo"`)) ||
		bytes.Contains(body, []byte(`"model":"Reolink`)) ||
		bytes.Contains(bytes.ToLower(body), []byte(`reolink`)) {
		return "reolink"
	}
	return ""
}

// probeHikvision checks the ISAPI System/deviceInfo endpoint.
func probeHikvision(ip, user, pass string) string {
	url := fmt.Sprintf("http://%s/ISAPI/System/deviceInfo", ip)
	body, err := httpGetBodyWithAuth(url, user, pass, 3*time.Second)
	if err != nil {
		return ""
	}
	if bytes.Contains(body, []byte(`<manufacturer>Hikvision</manufacturer>`)) ||
		bytes.Contains(body, []byte(`<deviceType>`)) {
		return "hikvision"
	}
	return ""
}

// probeONVIF queries ONVIF GetDeviceInformation and looks for known manufacturers.
func probeONVIF(ip, user, pass string) string {
	soap := `<?xml version="1.0" encoding="UTF-8"?>
<Envelope xmlns="http://www.w3.org/2003/05/soap-envelope">
  <Body>
    <GetDeviceInformation xmlns="http://www.onvif.org/ver10/device/wsdl"/>
  </Body>
</Envelope>`

	url := fmt.Sprintf("http://%s/onvif/device_service", ip)
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(soap))
	if err != nil {
		return ""
	}
	req.Header.Set("Content-Type", "application/soap+xml; charset=utf-8")
	req.Header.Set("SOAPAction", "http://www.onvif.org/ver10/device/wsdl/GetDeviceInformation")
	if user != "" {
		req.SetBasicAuth(user, pass)
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if err != nil {
		return ""
	}
	bodyLower := bytes.ToLower(body)
	if bytes.Contains(bodyLower, []byte(`<manufacturer>reolink`)) {
		return "reolink"
	}
	if bytes.Contains(bodyLower, []byte(`<manufacturer>hikvision`)) {
		return "hikvision"
	}
	if bytes.Contains(bodyLower, []byte(`<manufacturer>`)) {
		return "onvif"
	}
	return ""
}

// httpPostBody performs a simple HTTP POST with a plain text body and returns the response.
func httpPostBody(url, payload string, timeout time.Duration) ([]byte, error) {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Post(url, "application/json", strings.NewReader(payload))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 8192))
}

// httpGetBodyWithAuth performs an HTTP GET with digest/basic auth fallback.
func httpGetBodyWithAuth(url, user, pass string, timeout time.Duration) ([]byte, error) {
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if user != "" {
		req.SetBasicAuth(user, pass)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 8192))
}

// FindGo2rtcURL attempts to locate a reachable go2rtc instance.
// frigateURL is optional and used to derive a candidate like http://frigate:1984.
// It returns the first URL that responds to GET /api with a version field.
func FindGo2rtcURL(frigateURL string) string {
	candidates := go2rtcCandidates(frigateURL)
	for _, u := range candidates {
		if u == "" {
			continue
		}
		if isGo2rtcURL(u) {
			return u
		}
	}
	return ""
}

// go2rtcCandidates returns possible go2rtc URLs in priority order.
func go2rtcCandidates(frigateURL string) []string {
	candidates := []string{
		os.Getenv("CAMSPEAK_GO2RTC_URL"),
	}
	if frigateURL != "" {
		if u, err := url.Parse(frigateURL); err == nil && u.Host != "" {
			host := u.Hostname()
			candidates = append(candidates, fmt.Sprintf("http://%s:1984", host))
		}
	}
	candidates = append(candidates,
		"http://frigate:1984",
		"http://localhost:1984",
		"http://go2rtc:1984",
	)
	return candidates
}

// isGo2rtcURL checks whether url hosts a go2rtc API by hitting GET /api.
func isGo2rtcURL(url string) bool {
	apiURL := strings.TrimRight(url, "/") + "/api"
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(apiURL)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if err != nil {
		return false
	}
	var info struct {
		Version string `json:"version"`
	}
	return json.Unmarshal(body, &info) == nil && info.Version != ""
}

// ListGo2rtcStreams queries go2rtc for all configured stream names.
// Returns a map of stream name → raw source string (first producer URL).
func ListGo2rtcStreams(go2rtcURL string) (map[string]string, error) {
	apiURL := strings.TrimRight(go2rtcURL, "/") + "/api/streams"
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("querying go2rtc streams: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("go2rtc returned HTTP %d for /api/streams", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("reading go2rtc streams response: %w", err)
	}
	// go2rtc /api/streams returns: {"stream_name": {"producers": [{"url": "..."}], "consumers": [...]}}
	var raw map[string]struct {
		Producers []struct {
			URL    string `json:"url"`
			Source string `json:"source"`
		} `json:"producers"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parsing go2rtc streams JSON: %w", err)
	}
	out := make(map[string]string, len(raw))
	for name, info := range raw {
		src := ""
		if len(info.Producers) > 0 {
			// Prefer "url" (the original source URL), fall back to "source"
			if info.Producers[0].URL != "" {
				src = info.Producers[0].URL
			} else {
				src = info.Producers[0].Source
			}
		}
		out[name] = src
	}
	return out, nil
}

// Go2rtcStreamExists checks whether a stream with the given name exists in go2rtc.
func Go2rtcStreamExists(go2rtcURL, streamName string) bool {
	streams, err := ListGo2rtcStreams(go2rtcURL)
	if err != nil {
		return false
	}
	_, ok := streams[streamName]
	return ok
}
