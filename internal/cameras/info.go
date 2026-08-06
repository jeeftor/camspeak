// Package cameras provides audio streaming clients for camera types.
package cameras

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/icholy/digest"

	"github.com/jeeftor/camspeak/internal/config"
)

// CameraInfo is a vendor-neutral summary of a camera's device info and
// streaming configuration. Returned by QueryCameraInfo.
type CameraInfo struct {
	Type    string                 `json:"type"` // "hikvision", "onvif", etc.
	Online  bool                   `json:"online"`
	Device  DeviceInfo             `json:"device"`
	Network *NetworkInfo           `json:"network,omitempty"`
	Streams []StreamInfo           `json:"streams"`
	Raw     map[string]interface{} `json:"raw,omitempty"` // vendor-specific extras
	Errors  []string               `json:"errors,omitempty"`
}

// DeviceInfo holds identifying information about the camera hardware.
type DeviceInfo struct {
	Manufacturer string `json:"manufacturer"`
	Model        string `json:"model"`
	Firmware     string `json:"firmware"`
	Serial       string `json:"serial"`
	DeviceType   string `json:"device_type,omitempty"`
	Hardware     string `json:"hardware,omitempty"`
}

// NetworkInfo holds the camera's network configuration.
type NetworkInfo struct {
	IP      string `json:"ip"`
	MAC     string `json:"mac"`
	Gateway string `json:"gateway,omitempty"`
	Subnet  string `json:"subnet,omitempty"`
	DNS     string `json:"dns,omitempty"`
}

// StreamInfo describes a single video+audio stream channel.
type StreamInfo struct {
	Channel int        `json:"channel"`
	Name    string     `json:"name"`
	Video   *VideoInfo `json:"video,omitempty"`
	Audio   *AudioInfo `json:"audio,omitempty"`
}

// VideoInfo holds video encoder settings for a stream.
type VideoInfo struct {
	Codec       string `json:"codec"`
	Resolution  string `json:"resolution"` // "1920x1080"
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Framerate   int    `json:"framerate"` // fps
	Bitrate     int    `json:"bitrate"`   // kbps
	BitrateType string `json:"bitrate_type,omitempty"`
	GOP         int    `json:"gop,omitempty"`
	Profile     string `json:"profile,omitempty"`
}

// AudioInfo holds audio encoder settings for a stream.
type AudioInfo struct {
	Codec      string `json:"codec"`
	SampleRate int    `json:"sample_rate"` // Hz
	Bitrate    int    `json:"bitrate"`     // kbps
	Channels   int    `json:"channels"`
}

// QueryCameraInfo dispatches to the appropriate vendor query based on camera type.
// Supported types: "hikvision" (ISAPI), "onvif" (SOAP), "reolink" (ISAPI if
// Hikvision-compatible, otherwise ONVIF SOAP fallback).
func QueryCameraInfo(cam config.CameraConfig) (CameraInfo, error) {
	switch cam.Type {
	case "hikvision":
		return queryHikvisionInfo(cam.IP, cam.User, cam.Pass)
	case "onvif":
		return queryOnvifInfo(cam.IP, cam.User, cam.Pass)
	case "reolink":
		// Reolink cameras often expose ONVIF too — try ONVIF SOAP first.
		info, err := queryOnvifInfo(cam.IP, cam.User, cam.Pass)
		if err == nil && info.Online {
			return info, nil
		}
		return info, fmt.Errorf("reolink settings query not supported (ONVIF fallback failed: %w)", err)
	case "go2rtc":
		return CameraInfo{Type: "go2rtc", Online: false},
			fmt.Errorf("go2rtc cameras do not expose device settings")
	default:
		return CameraInfo{Type: cam.Type},
			fmt.Errorf("unsupported camera type %q for settings query", cam.Type)
	}
}

// ---------------------------------------------------------------------------
// Hikvision ISAPI
// ---------------------------------------------------------------------------

var xmlnsRe = regexp.MustCompile(`\sxmlns="[^"]*"`)

// stripXMLNS removes default xmlns attributes so encoding/xml matches by local name.
func stripXMLNS(data []byte) []byte {
	return xmlnsRe.ReplaceAll(data, nil)
}

// hikDeviceInfoXML maps the ISAPI /ISAPI/System/deviceInfo response.
type hikDeviceInfoXML struct {
	XMLName         xml.Name `xml:"DeviceInfo"`
	DeviceName      string   `xml:"deviceName"`
	Model           string   `xml:"model"`
	SerialNumber    string   `xml:"serialNumber"`
	MacAddress      string   `xml:"macAddress"`
	FirmwareVersion string   `xml:"firmwareVersion"`
	DeviceType      string   `xml:"deviceType"`
	HardwareVersion string   `xml:"hardwareVersion"`
	Manufacturer    string   `xml:"manufacturer"`
}

// hikNetworkInfoXML maps the ISAPI /ISAPI/System/networkInfo response.
type hikNetworkInfoXML struct {
	XMLName        xml.Name `xml:"NetworkInfo"`
	IPAddress      string   `xml:"ipAddress"`
	SubnetMask     string   `xml:"subnetMask"`
	DefaultGateway string   `xml:"defaultGateway"`
	MacAddress     string   `xml:"macAddress"`
	DNSServer      string   `xml:"dnsServer"`
}

// hikStreamingChannelListXML maps the ISAPI /ISAPI/Streaming/channels response.
type hikStreamingChannelListXML struct {
	XMLName          xml.Name              `xml:"StreamingChannelList"`
	StreamingChannel []hikStreamingChannel `xml:"StreamingChannel"`
}

type hikStreamingChannel struct {
	ID          int    `xml:"id"`
	ChannelName string `xml:"channelName"`
	Transport   string `xml:"transport"`
	Video       *struct {
		Enabled                 bool   `xml:"enabled"`
		VideoCodecType          string `xml:"videoCodecType"`
		VideoResolutionWidth    int    `xml:"videoResolutionWidth"`
		VideoResolutionHeight   int    `xml:"videoResolutionHeight"`
		VideoQualityControlType string `xml:"videoQualityControlType"`
		ConstantBitRate         int    `xml:"constantBitRate"`
		MaxFrameRate            int    `xml:"maxFrameRate"` // centi-fps (2500 = 25fps)
		KeyFrameInterval        int    `xml:"keyFrameInterval"`
		H264Profile             string `xml:"H264Profile"`
		H265Profile             string `xml:"H265Profile"`
	} `xml:"Video"`
	Audio *struct {
		Enabled         bool   `xml:"enabled"`
		AudioCodecType  string `xml:"audioCodecType"`
		AudioBitRate    int    `xml:"audioBitRate"`
		AudioSampleRate int    `xml:"audioSampleRate"`
	} `xml:"Audio"`
}

// queryHikvisionInfo queries ISAPI endpoints for device info, streaming config, and network.
func queryHikvisionInfo(ip, user, pass string) (CameraInfo, error) {
	info := CameraInfo{Type: "hikvision", Raw: map[string]interface{}{}}

	transport := &digest.Transport{Username: user, Password: pass}
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}

	// 1. Device info
	if body, err := isapiGet(client, ip, "/ISAPI/System/deviceInfo"); err != nil {
		info.Errors = append(info.Errors, fmt.Sprintf("deviceInfo: %s", err))
	} else {
		var dev hikDeviceInfoXML
		if err := xml.Unmarshal(stripXMLNS(body), &dev); err != nil {
			info.Errors = append(info.Errors, fmt.Sprintf("deviceInfo parse: %s", err))
		} else {
			info.Online = true
			info.Device = DeviceInfo{
				Manufacturer: dev.Manufacturer,
				Model:        dev.Model,
				Firmware:     dev.FirmwareVersion,
				Serial:       dev.SerialNumber,
				DeviceType:   dev.DeviceType,
				Hardware:     dev.HardwareVersion,
			}
			if dev.MacAddress != "" && info.Network == nil {
				info.Network = &NetworkInfo{MAC: dev.MacAddress}
			}
		}
	}

	// 2. Streaming channels
	if body, err := isapiGet(client, ip, "/ISAPI/Streaming/channels"); err != nil {
		info.Errors = append(info.Errors, fmt.Sprintf("streaming: %s", err))
	} else {
		var list hikStreamingChannelListXML
		if err := xml.Unmarshal(stripXMLNS(body), &list); err != nil {
			info.Errors = append(info.Errors, fmt.Sprintf("streaming parse: %s", err))
		} else {
			info.Online = true
			for _, ch := range list.StreamingChannel {
				si := StreamInfo{
					Channel: ch.ID,
					Name:    ch.ChannelName,
				}
				if ch.Video != nil {
					fps := ch.Video.MaxFrameRate / 100
					if fps == 0 && ch.Video.MaxFrameRate > 0 {
						fps = 1 // avoid 0 for sub-1fps values
					}
					profile := ch.Video.H264Profile
					if profile == "" {
						profile = ch.Video.H265Profile
					}
					si.Video = &VideoInfo{
						Codec: ch.Video.VideoCodecType,
						Resolution: fmt.Sprintf(
							"%dx%d",
							ch.Video.VideoResolutionWidth,
							ch.Video.VideoResolutionHeight,
						),
						Width:       ch.Video.VideoResolutionWidth,
						Height:      ch.Video.VideoResolutionHeight,
						Framerate:   fps,
						Bitrate:     ch.Video.ConstantBitRate,
						BitrateType: ch.Video.VideoQualityControlType,
						GOP:         ch.Video.KeyFrameInterval,
						Profile:     profile,
					}
				}
				if ch.Audio != nil {
					si.Audio = &AudioInfo{
						Codec:      ch.Audio.AudioCodecType,
						SampleRate: ch.Audio.AudioSampleRate,
						Bitrate:    ch.Audio.AudioBitRate,
						Channels:   1, // ISAPI doesn't expose channel count; default to 1
					}
				}
				info.Streams = append(info.Streams, si)
			}
		}
	}

	// 3. Network info
	if body, err := isapiGet(client, ip, "/ISAPI/System/networkInfo"); err != nil {
		info.Errors = append(info.Errors, fmt.Sprintf("networkInfo: %s", err))
	} else {
		var net hikNetworkInfoXML
		if err := xml.Unmarshal(stripXMLNS(body), &net); err != nil {
			info.Errors = append(info.Errors, fmt.Sprintf("networkInfo parse: %s", err))
		} else {
			info.Online = true
			if info.Network == nil {
				info.Network = &NetworkInfo{}
			}
			info.Network.IP = net.IPAddress
			info.Network.MAC = net.MacAddress
			info.Network.Gateway = net.DefaultGateway
			info.Network.Subnet = net.SubnetMask
			info.Network.DNS = net.DNSServer
		}
	}

	if !info.Online && len(info.Errors) > 0 {
		return info, fmt.Errorf("camera unreachable: %s", strings.Join(info.Errors, "; "))
	}
	return info, nil
}

// isapiGet performs an authenticated GET to an ISAPI endpoint and returns the body.
func isapiGet(client *http.Client, ip, path string) ([]byte, error) {
	url := fmt.Sprintf("http://%s%s", ip, path)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("authentication failed (HTTP 401)")
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit
}

// ---------------------------------------------------------------------------
// ONVIF SOAP
// ---------------------------------------------------------------------------

// onvifGetDeviceInformationResponse maps the SOAP response for GetDeviceInformation.
type onvifGetDeviceInformationResponse struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    struct {
		GetDeviceInformationResponse struct {
			Manufacturer    string `xml:"Manufacturer"`
			Model           string `xml:"Model"`
			FirmwareVersion string `xml:"FirmwareVersion"`
			SerialNumber    string `xml:"SerialNumber"`
			HardwareId      string `xml:"HardwareId"`
		} `xml:"GetDeviceInformationResponse"`
		Fault *struct {
			Code struct {
				Value string `xml:"Value"`
			} `xml:"Code"`
			Reason struct {
				Text string `xml:"Text"`
			} `xml:"Reason"`
		} `xml:"Fault"`
	} `xml:"Body"`
}

// onvifGetVideoEncoderConfigurationsResponse maps the SOAP response.
type onvifGetVideoEncoderConfigurationsResponse struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    struct {
		Configurations []struct {
			Name       string `xml:"Name"`
			Encoding   string `xml:"Encoding"`
			Resolution struct {
				Width  int `xml:"Width"`
				Height int `xml:"Height"`
			} `xml:"Resolution"`
			Quality     int `xml:"Quality"`
			RateControl struct {
				FrameRateLimit   int `xml:"FrameRateLimit"`
				EncodingInterval int `xml:"EncodingInterval"`
				BitrateLimit     int `xml:"BitrateLimit"`
			} `xml:"RateControl"`
			H264 struct {
				GovLength   int    `xml:"GovLength"`
				H264Profile string `xml:"H264Profile"`
			} `xml:"H264"`
			H265 struct {
				GovLength   int    `xml:"GovLength"`
				H265Profile string `xml:"H265Profile"`
			} `xml:"H265"`
			MPEG4 struct {
				GovLength    int    `xml:"GovLength"`
				Mpeg4Profile string `xml:"Mpeg4Profile"`
			} `xml:"MPEG4"`
		} `xml:"Configurations"`
		Fault *struct {
			Code struct {
				Value string `xml:"Value"`
			} `xml:"Code"`
			Reason struct {
				Text string `xml:"Text"`
			} `xml:"Reason"`
		} `xml:"Fault"`
	} `xml:"Body"`
}

// onvifGetAudioEncoderConfigurationsResponse maps the SOAP response.
type onvifGetAudioEncoderConfigurationsResponse struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    struct {
		Configurations []struct {
			Name       string `xml:"Name"`
			Encoding   string `xml:"Encoding"`
			Bitrate    int    `xml:"Bitrate"`
			SampleRate int    `xml:"SampleRate"`
		} `xml:"Configurations"`
		Fault *struct {
			Code struct {
				Value string `xml:"Value"`
			} `xml:"Code"`
			Reason struct {
				Text string `xml:"Text"`
			} `xml:"Reason"`
		} `xml:"Fault"`
	} `xml:"Body"`
}

// onvifGetCapabilitiesResponse extracts the media service XAddr.
type onvifGetCapabilitiesResponse struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    struct {
		GetCapabilitiesResponse struct {
			Capabilities struct {
				Media struct {
					XAddr string `xml:"XAddr"`
				} `xml:"Media"`
			} `xml:"Capabilities"`
			Fault *struct {
				Code struct {
					Value string `xml:"Value"`
				} `xml:"Code"`
				Reason struct {
					Text string `xml:"Text"`
				} `xml:"Reason"`
			} `xml:"Fault"`
		} `xml:"GetCapabilitiesResponse"`
	} `xml:"Body"`
}

// queryOnvifInfo queries ONVIF SOAP endpoints for device info and encoder configs.
func queryOnvifInfo(ip, user, pass string) (CameraInfo, error) {
	info := CameraInfo{Type: "onvif", Raw: map[string]interface{}{}}
	deviceURL := fmt.Sprintf("http://%s/onvif/device_service", ip)

	// 1. GetDeviceInformation
	soap := onvifSoapEnvelope(`<GetDeviceInformation xmlns="http://www.onvif.org/ver10/device/wsdl"/>`)
	body, err := onvifSoapRequest(deviceURL, soap, user, pass,
		"http://www.onvif.org/ver10/device/wsdl/GetDeviceInformation")
	if err != nil {
		info.Errors = append(info.Errors, fmt.Sprintf("GetDeviceInformation: %s", err))
		return info, fmt.Errorf("GetDeviceInformation: %w", err)
	}

	var devResp onvifGetDeviceInformationResponse
	if err := xml.Unmarshal(stripXMLNS(body), &devResp); err != nil {
		info.Errors = append(info.Errors, fmt.Sprintf("deviceInfo parse: %s", err))
		return info, fmt.Errorf("parse deviceInfo: %w", err)
	}
	if devResp.Body.Fault != nil {
		info.Errors = append(
			info.Errors,
			fmt.Sprintf("deviceInfo fault: %s", devResp.Body.Fault.Reason.Text),
		)
		return info, fmt.Errorf("deviceInfo fault: %s", devResp.Body.Fault.Reason.Text)
	}

	info.Online = true
	info.Device = DeviceInfo{
		Manufacturer: devResp.Body.GetDeviceInformationResponse.Manufacturer,
		Model:        devResp.Body.GetDeviceInformationResponse.Model,
		Firmware:     devResp.Body.GetDeviceInformationResponse.FirmwareVersion,
		Serial:       devResp.Body.GetDeviceInformationResponse.SerialNumber,
		Hardware:     devResp.Body.GetDeviceInformationResponse.HardwareId,
	}

	// 2. GetCapabilities to find the media service URL
	soap = onvifSoapEnvelope(
		`<GetCapabilities xmlns="http://www.onvif.org/ver10/device/wsdl">` +
			`<Category>All</Category></GetCapabilities>`,
	)
	capBody, err := onvifSoapRequest(deviceURL, soap, user, pass,
		"http://www.onvif.org/ver10/device/wsdl/GetCapabilities")
	mediaURL := deviceURL // fallback: try device service for media commands too
	if err == nil {
		var capResp onvifGetCapabilitiesResponse
		if err := xml.Unmarshal(stripXMLNS(capBody), &capResp); err == nil {
			if capResp.Body.GetCapabilitiesResponse.Capabilities.Media.XAddr != "" {
				mediaURL = capResp.Body.GetCapabilitiesResponse.Capabilities.Media.XAddr
			}
		}
	}

	// 3. GetVideoEncoderConfigurations
	soap = onvifSoapEnvelope(
		`<GetVideoEncoderConfigurations xmlns="http://www.onvif.org/ver10/media/wsdl"/>`,
	)
	vidBody, err := onvifSoapRequest(mediaURL, soap, user, pass,
		"http://www.onvif.org/ver10/media/wsdl/GetVideoEncoderConfigurations")
	if err != nil {
		info.Errors = append(info.Errors, fmt.Sprintf("GetVideoEncoderConfigurations: %s", err))
	} else {
		var vidResp onvifGetVideoEncoderConfigurationsResponse
		if err := xml.Unmarshal(stripXMLNS(vidBody), &vidResp); err != nil {
			info.Errors = append(info.Errors, fmt.Sprintf("videoEncoder parse: %s", err))
		} else if vidResp.Body.Fault != nil {
			info.Errors = append(
				info.Errors,
				fmt.Sprintf("videoEncoder fault: %s", vidResp.Body.Fault.Reason.Text),
			)
		} else {
			for i, cfg := range vidResp.Body.Configurations {
				si := StreamInfo{
					Channel: i + 1,
					Name:    cfg.Name,
				}
				profile := cfg.H264.H264Profile
				if profile == "" {
					profile = cfg.H265.H265Profile
				}
				gop := cfg.H264.GovLength
				if gop == 0 {
					gop = cfg.H265.GovLength
				}
				if gop == 0 {
					gop = cfg.MPEG4.GovLength
				}
				si.Video = &VideoInfo{
					Codec:      cfg.Encoding,
					Resolution: fmt.Sprintf("%dx%d", cfg.Resolution.Width, cfg.Resolution.Height),
					Width:      cfg.Resolution.Width,
					Height:     cfg.Resolution.Height,
					Framerate:  cfg.RateControl.FrameRateLimit,
					Bitrate:    cfg.RateControl.BitrateLimit,
					GOP:        gop,
					Profile:    profile,
				}
				info.Streams = append(info.Streams, si)
			}
		}
	}

	// 4. GetAudioEncoderConfigurations
	soap = onvifSoapEnvelope(
		`<GetAudioEncoderConfigurations xmlns="http://www.onvif.org/ver10/media/wsdl"/>`,
	)
	audBody, err := onvifSoapRequest(mediaURL, soap, user, pass,
		"http://www.onvif.org/ver10/media/wsdl/GetAudioEncoderConfigurations")
	if err != nil {
		info.Errors = append(info.Errors, fmt.Sprintf("GetAudioEncoderConfigurations: %s", err))
	} else {
		var audResp onvifGetAudioEncoderConfigurationsResponse
		if err := xml.Unmarshal(stripXMLNS(audBody), &audResp); err != nil {
			info.Errors = append(info.Errors, fmt.Sprintf("audioEncoder parse: %s", err))
		} else if audResp.Body.Fault != nil {
			info.Errors = append(
				info.Errors,
				fmt.Sprintf("audioEncoder fault: %s", audResp.Body.Fault.Reason.Text),
			)
		} else {
			// Merge audio configs into existing streams, or append as new entries.
			for i, cfg := range audResp.Body.Configurations {
				ai := &AudioInfo{
					Codec:      cfg.Encoding,
					Bitrate:    cfg.Bitrate,
					SampleRate: cfg.SampleRate,
					Channels:   1,
				}
				if i < len(info.Streams) {
					info.Streams[i].Audio = ai
				} else {
					info.Streams = append(info.Streams, StreamInfo{
						Channel: i + 1,
						Name:    cfg.Name,
						Audio:   ai,
					})
				}
			}
		}
	}

	if !info.Online {
		return info, fmt.Errorf("camera unreachable: %s", strings.Join(info.Errors, "; "))
	}
	return info, nil
}

// onvifSoapEnvelope wraps innerBody in a standard SOAP envelope.
func onvifSoapEnvelope(innerBody string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>%s</s:Body>
</s:Envelope>`, innerBody)
}

// onvifSoapRequest sends a SOAP request to the given URL with optional auth.
func onvifSoapRequest(url, soapBody, user, pass, action string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(soapBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/soap+xml; charset=utf-8")
	if action != "" {
		req.Header.Set("SOAPAction", action)
	}
	if user != "" {
		req.SetBasicAuth(user, pass)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("authentication failed (HTTP 401)")
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}
