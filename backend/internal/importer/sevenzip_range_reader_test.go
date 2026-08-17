package importer

import (
	"bytes"
	"container/list"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"io"
	"os"
	"testing"

	metapb "novastream/internal/nzb/metadata/proto"
)

func newMemoryMultiPart7zReader(t *testing.T, parts ...[]byte) *multiPart7zReader {
	t.Helper()
	files := make([]ParsedFile, len(parts))
	offsets := make([]int64, len(parts))
	byName := make(map[string][]byte, len(parts))
	var total int64
	for i, part := range parts {
		name := string(rune('a' + i))
		files[i] = ParsedFile{Filename: name, Size: int64(len(part))}
		offsets[i] = total
		total += int64(len(part))
		byName[name] = part
	}
	r := &multiPart7zReader{
		sortFiles:   files,
		partOffsets: offsets,
		totalSize:   total,
		pageSize:    4,
		maxPages:    2,
		pages:       make(map[int64]*list.Element),
		lru:         list.New(),
	}
	r.readPartAtFn = func(part *ParsedFile, p []byte, off int64) (int, error) {
		n := copy(p, byName[part.Filename][off:])
		if n != len(p) {
			return n, io.EOF
		}
		return n, nil
	}
	return r
}

func TestMultiPart7zReaderReadsAcrossParts(t *testing.T) {
	r := newMemoryMultiPart7zReader(t, []byte("abc"), []byte("defgh"), []byte("ijk"))
	got := make([]byte, 7)
	n, err := r.ReadAt(got, 2)
	if err != nil {
		t.Fatalf("ReadAt: %v", err)
	}
	if n != len(got) || string(got) != "cdefghi" {
		t.Fatalf("ReadAt = %q, %d; want %q, %d", got, n, "cdefghi", len(got))
	}
}

func TestMultiPart7zReaderBoundsCacheAndEOF(t *testing.T) {
	r := newMemoryMultiPart7zReader(t, []byte("abcdefghijkl"))
	for _, off := range []int64{0, 4, 8} {
		buf := make([]byte, 1)
		if _, err := r.ReadAt(buf, off); err != nil {
			t.Fatalf("ReadAt(%d): %v", off, err)
		}
	}
	if got := r.lru.Len(); got != 2 {
		t.Fatalf("cached pages = %d, want 2", got)
	}

	buf := make([]byte, 4)
	n, err := r.ReadAt(buf, 10)
	if err != io.EOF || n != 2 || string(buf[:n]) != "kl" {
		t.Fatalf("tail ReadAt = %q, %d, %v; want %q, 2, EOF", buf[:n], n, err, "kl")
	}
}

func TestParse7zHeadersUsesPasswordForEncryptedHeader(t *testing.T) {
	data, err := os.ReadFile("testdata/password-store.7z")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parse7zHeaders(bytes.NewReader(data), int64(len(data)), "wrong"); err == nil {
		t.Fatal("parse7zHeaders with wrong password unexpectedly succeeded")
	}
	info, err := parse7zHeaders(bytes.NewReader(data), int64(len(data)), "test-password")
	if err != nil {
		t.Fatalf("parse7zHeaders: %v", err)
	}
	if !info.IsUncompressed || len(info.Files) != 1 || info.Files[0].Name != "testdata/password-store.txt" {
		t.Fatalf("unexpected archive info: %#v", info)
	}
	if len(info.Files[0].AesKey) != 32 || len(info.Files[0].AesIV) != 16 || info.Files[0].PackedSize != 32 {
		t.Fatalf("unexpected AES metadata: key=%d iv=%d packed=%d", len(info.Files[0].AesKey), len(info.Files[0].AesIV), info.Files[0].PackedSize)
	}
	entry := info.Files[0]
	ciphertext := append([]byte(nil), data[entry.PackedOffset:entry.PackedOffset+entry.PackedSize]...)
	block, err := aes.NewCipher(entry.AesKey)
	if err != nil {
		t.Fatal(err)
	}
	cipher.NewCBCDecrypter(block, entry.AesIV).CryptBlocks(ciphertext, ciphertext)
	if got, want := string(ciphertext[:entry.UncompressedSize]), "bounded 7z password test\n"; got != want {
		t.Fatalf("decrypted payload = %q, want %q", got, want)
	}
}

func TestSevenZipAESMetadataStoresDerivedMaterial(t *testing.T) {
	content := sevenZipContent{
		Size:   25,
		AesKey: bytes.Repeat([]byte{0x2a}, 32),
		AesIV:  bytes.Repeat([]byte{0x7b}, 16),
	}
	meta := (&sevenZipProcessor{}).CreateFileMetadataFrom7zContent(content, "release.nzb")
	if meta.Encryption != metapb.Encryption_HEADERS {
		t.Fatalf("encryption = %v, want HEADERS", meta.Encryption)
	}
	if meta.Password != base64.StdEncoding.EncodeToString(content.AesKey) || meta.Salt != base64.StdEncoding.EncodeToString(content.AesIV) {
		t.Fatal("derived AES key/IV were not persisted in metadata")
	}
}
