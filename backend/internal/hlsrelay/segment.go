package hlsrelay

import "bytes"

// imageTransportStreamOffset recognizes the WebP camouflage used by some live
// sports CDNs. Require six consecutive MPEG-TS packets before discarding bytes;
// ordinary images, encrypted segments and other media must remain untouched.
func imageTransportStreamOffset(prefix []byte) int {
	if len(prefix) < 12 || !bytes.Equal(prefix[:4], []byte("RIFF")) || !bytes.Equal(prefix[8:12], []byte("WEBP")) {
		return 0
	}
	for offset := 12; offset < 4096 && offset+5*188 < len(prefix); offset++ {
		valid := true
		for packet := 0; packet < 6; packet++ {
			if prefix[offset+packet*188] != 0x47 {
				valid = false
				break
			}
		}
		if valid {
			return offset
		}
	}
	return 0
}
