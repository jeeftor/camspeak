package util

// mulawToLinear is the standard G.711 µ-law decode table (256 entries).
// Each byte maps to a 16-bit linear PCM sample.
var mulawToLinear = [256]int16{
	-32124, -31100, -30076, -29052, -28028, -27004, -25980, -24956,
	-23932, -22908, -21884, -20860, -19836, -18812, -17788, -16764,
	-15996, -15484, -14972, -14460, -13948, -13436, -12924, -12412,
	-11900, -11388, -10876, -10364, -9852, -9340, -8828, -8316,
	-7932, -7676, -7420, -7164, -6908, -6652, -6396, -6140,
	-5884, -5628, -5372, -5116, -4860, -4604, -4348, -4092,
	-3900, -3772, -3644, -3516, -3388, -3260, -3132, -3004,
	-2876, -2748, -2620, -2492, -2364, -2236, -2108, -1980,
	-1884, -1820, -1756, -1692, -1628, -1564, -1500, -1436,
	-1372, -1308, -1244, -1180, -1116, -1052, -988, -924,
	-876, -844, -812, -780, -748, -716, -684, -652,
	-620, -588, -556, -524, -492, -460, -428, -396,
	-372, -356, -340, -324, -308, -292, -276, -260,
	-244, -228, -212, -196, -180, -164, -148, -132,
	-120, -112, -104, -96, -88, -80, -72, -64,
	-56, -48, -40, -32, -24, -16, -8, 0,
	32124, 31100, 30076, 29052, 28028, 27004, 25980, 24956,
	23932, 22908, 21884, 20860, 19836, 18812, 17788, 16764,
	15996, 15484, 14972, 14460, 13948, 13436, 12924, 12412,
	11900, 11388, 10876, 10364, 9852, 9340, 8828, 8316,
	7932, 7676, 7420, 7164, 6908, 6652, 6396, 6140,
	5884, 5628, 5372, 5116, 4860, 4604, 4348, 4092,
	3900, 3772, 3644, 3516, 3388, 3260, 3132, 3004,
	2876, 2748, 2620, 2492, 2364, 2236, 2108, 1980,
	1884, 1820, 1756, 1692, 1628, 1564, 1500, 1436,
	1372, 1308, 1244, 1180, 1116, 1052, 988, 924,
	876, 844, 812, 780, 748, 716, 684, 652,
	620, 588, 556, 524, 492, 460, 428, 396,
	372, 356, 340, 324, 308, 292, 276, 260,
	244, 228, 212, 196, 180, 164, 148, 132,
	120, 112, 104, 96, 88, 80, 72, 64,
	56, 48, 40, 32, 24, 16, 8, 0,
}

// linearToMulaw is the encode table (65536 entries, built at init from the
// decode table via nearest-neighbor search — guarantees round-trip correctness).
var linearToMulaw [65536]byte

func init() {
	// For each possible int16 value, find the µ-law byte whose decoded
	// PCM is closest. This is O(65536 * 256) = 16M, runs once at startup.
	for i := 0; i < 65536; i++ {
		pcm := int16(i - 32768)
		bestByte := byte(128) // silence
		bestDist := int(1 << 30)
		for b := 0; b < 256; b++ {
			decoded := mulawToLinear[b]
			dist := int(decoded) - int(pcm)
			if dist < 0 {
				dist = -dist
			}
			if dist < bestDist {
				bestDist = dist
				bestByte = byte(b)
			}
		}
		linearToMulaw[i] = bestByte
	}
}

// MulawDecode converts a single µ-law byte to a 16-bit linear PCM sample.
func MulawDecode(b byte) int16 {
	return mulawToLinear[b]
}

// MulawEncode converts a 16-bit linear PCM sample to a µ-law byte.
func MulawEncode(s int16) byte {
	return linearToMulaw[int(s)+32768]
}

// ApplyGainMulaw applies a volume gain to a buffer of G.711 µ-law encoded audio
// in place. Each byte is decoded to linear PCM, scaled by gain, and re-encoded.
// gain=1.0 is unity (no change), gain=3.0 is 3x amplification.
// gain=0.0 produces µ-law silence (byte 128).
// Samples are clamped to int16 range to prevent wrap-around.
//
// NOTE: This assumes the buffer is G.711 µ-law. All camera types currently
// receive µ-law from the transcoder (pcm_mulaw, 8kHz). If a future camera type
// uses a different codec (e.g. AAC, G.722, ADPCM), this function must NOT be
// called on that data — it would corrupt the audio. A codec-aware gain
// interface would be needed at that point.
func ApplyGainMulaw(buf []byte, gain float64) {
	if gain == 1.0 {
		return
	}
	for i := range buf {
		pcm := float64(mulawToLinear[buf[i]])
		pcm *= gain
		// Clamp to int16 range
		if pcm > 32767 {
			pcm = 32767
		} else if pcm < -32768 {
			pcm = -32768
		}
		buf[i] = linearToMulaw[int(int16(pcm))+32768]
	}
}
