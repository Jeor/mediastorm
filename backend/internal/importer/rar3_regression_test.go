package importer

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"log/slog"
	"testing"

	"github.com/javi11/rardecode/v2"
	"github.com/stretchr/testify/require"
	metapb "novastream/internal/nzb/metadata/proto"
)

// Build real RAR3 block headers, including CRC and PACK_SIZE (the LONG_BLOCK
// field itself, not an additional copy). Optional end fields vary trailer size.
func rar3Block(kind byte, flags uint16, body []byte) []byte {
	h := make([]byte, 7+len(body))
	h[2] = kind
	binary.LittleEndian.PutUint16(h[3:], flags)
	binary.LittleEndian.PutUint16(h[5:], uint16(len(h)))
	copy(h[7:], body)
	binary.LittleEndian.PutUint16(h, uint16(crc32.ChecksumIEEE(h[2:])))
	return h
}

func TestRar3VolumeMappingPreservesEveryPayloadByte(t *testing.T) {
	for _, trailerExtra := range []int{0, 6, 11} {
		t.Run(fmt.Sprintf("trailer-%d", 7+trailerExtra), func(t *testing.T) {
			const name = "casualty.s28e12.hdtv.x264-river.mp4"
			payloads := [][]byte{bytes.Repeat([]byte("abcdefg"), 700), bytes.Repeat([]byte("hijklmn"), 500)}
			total := len(payloads[0]) + len(payloads[1])
			volumes := map[string][]byte{}
			var parsed []ParsedFile
			for i, payload := range payloads {
				path := fmt.Sprintf("episode.part%d.rar", i+1)
				mainFlags := uint16(0x11) // volume + new numbering
				if i == 0 {
					mainFlags |= 0x100
				}
				v := append([]byte("Rar!\x1a\x07\x00"), rar3Block(0x73, mainFlags, make([]byte, 6))...)
				body := make([]byte, 25)
				binary.LittleEndian.PutUint32(body, uint32(len(payload)))
				binary.LittleEndian.PutUint32(body[4:], uint32(total))
				body[8] = 3 // Unix
				body[17] = 20
				body[18] = 0x30 // stored
				binary.LittleEndian.PutUint16(body[19:], uint16(len(name)))
				flags := uint16(0x8000)
				if i == 0 {
					flags |= 2
				} else {
					flags |= 1
				}
				v = append(v, rar3Block(0x74, flags, append(body, []byte(name)...))...)
				v = append(v, payload...)
				endFlags := uint16(0)
				if i == 0 {
					endFlags |= 1
				}
				if trailerExtra >= 6 {
					endFlags |= 0xa
				} // data CRC and volume number
				v = append(v, rar3Block(0x7b, endFlags, make([]byte, trailerExtra))...)
				volumes[path] = v
				// Segment boundary inside payload exercises archive-to-article slicing.
				n := int64(len(v) / 2)
				parsed = append(parsed, ParsedFile{Filename: path, Size: int64(len(v)), Segments: []*metapb.SegmentData{seg(path+":a", n), seg(path+":b", int64(len(v))-n)}})
			}
			fsys := NewMemoryFileSystem(volumes)
			yes, err := isRar3Archive(fsys, parsed[0].Filename)
			require.NoError(t, err)
			require.True(t, yes)
			rp := &rarProcessor{maxWorkers: 2, log: slog.Default()}
			contents, err := rp.analyzeRarWithDecoder(context.Background(), fsys, parsed, parsed[0].Filename, "", nil)
			require.NoError(t, err)
			require.Len(t, contents, 1)
			require.Equal(t, name, contents[0].Filename)
			require.Equal(t, int64(total), contents[0].Size)
			articles := map[string][]byte{}
			for _, p := range parsed {
				n := len(volumes[p.Filename]) / 2
				articles[p.Filename+":a"] = volumes[p.Filename][:n]
				articles[p.Filename+":b"] = volumes[p.Filename][n:]
			}
			var reconstructed []byte
			for _, s := range contents[0].Segments {
				reconstructed = append(reconstructed, articles[s.Id][s.StartOffset:s.EndOffset+1]...)
			}
			require.Equal(t, bytes.Join(payloads, nil), reconstructed)
		})
	}
}

func TestDecodedRarRejectsIncompleteCoverage(t *testing.T) {
	rp := &rarProcessor{}
	files := []rardecode.ArchiveFileInfo{{Name: "movie.mp4", TotalUnpackedSize: 20, Parts: []rardecode.FilePartInfo{{Path: "movie.rar", DataOffset: 0, PackedSize: 10}}}}
	_, err := rp.convertDecodedFilesToRarContent(files, []ParsedFile{{Filename: "movie.rar", Segments: []*metapb.SegmentData{seg("a", 10)}}})
	require.ErrorContains(t, err, "covers 10 of 20 bytes")
	files[0].Parts[0].PackedSize = 20
	_, err = rp.convertDecodedFilesToRarContent(files, []ParsedFile{{Filename: "movie.rar", Segments: []*metapb.SegmentData{seg("a", 10)}}})
	require.ErrorContains(t, err, "covers 10 of 20 bytes")
	_, err = rp.convertDecodedFilesToRarContent(files, nil)
	require.ErrorContains(t, err, "not found in NZB")
}

func TestRarVersionDetection(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
		want bool
		bad  bool
	}{
		{"rar3", []byte("Rar!\x1a\x07\x00"), true, false},
		{"rar5", []byte("Rar!\x1a\x07\x01\x00"), false, false},
		{"short", []byte("Rar!"), false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := isRar3Archive(NewMemoryFileSystem(map[string][]byte{"test.rar": tc.data}), "test.rar")
			if tc.bad {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.want, got)
			}
		})
	}
}
