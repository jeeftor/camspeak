package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

// StreamInfo describes a single go2rtc stream.
type StreamInfo struct {
	Name   string `json:"name"`
	Video  string `json:"video"`  // codec e.g. "H265", "H264"; empty if no video
	Active bool   `json:"active"` // true if there's an active producer
	Source string `json:"source"` // RTSP URL or source path (redacted)
}

// go2rtcStreamsResponse matches the JSON structure from go2rtc's /api/streams.
type go2rtcStreamsResponse map[string]struct {
	Producers []struct {
		Medias []string `json:"medias"`
		URL    string   `json:"url"`
	} `json:"producers"`
}

// Streams handles GET /api/streams — lists all available go2rtc streams.
func (h *Handlers) Streams(c echo.Context) error {
	log := h.logger(c)
	if h.cfg.Go2rtcURL == "" {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "go2rtc URL not configured")
	}

	listURL := strings.TrimSuffix(h.cfg.Go2rtcURL, "/") + "/api/streams"
	req, err := http.NewRequestWithContext(c.Request().Context(), http.MethodGet, listURL, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Debug("streams: go2rtc API call failed", "err", err)
		return echo.NewHTTPError(http.StatusBadGateway, fmt.Sprintf("go2rtc API: %s", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return echo.NewHTTPError(
			http.StatusBadGateway,
			fmt.Sprintf("go2rtc returned HTTP %d", resp.StatusCode),
		)
	}

	var raw go2rtcStreamsResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return echo.NewHTTPError(
			http.StatusInternalServerError,
			fmt.Sprintf("decoding go2rtc response: %s", err),
		)
	}

	streams := make([]StreamInfo, 0, len(raw))
	for name, info := range raw {
		si := StreamInfo{Name: name}
		if len(info.Producers) > 0 {
			si.Active = true
			for _, media := range info.Producers[0].Medias {
				if strings.Contains(media, "video") {
					// e.g. "video, recvonly, H265" → extract codec
					parts := strings.Split(media, ",")
					if len(parts) >= 3 {
						si.Video = strings.TrimSpace(parts[len(parts)-1])
					}
				}
			}
		}
		streams = append(streams, si)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"status":  "ok",
		"streams": streams,
	})
}
