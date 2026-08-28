package aes

import (
	"bytes"
	"context"
	stdaes "crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"io"
	"testing"
	"time"

	"novastream/internal/nzb/utils"
)

func TestCipherDecryptsFullAndSeekedRanges(t *testing.T) {
	plain := bytes.Repeat([]byte("seekable-rar-payload-"), 257)
	key := []byte("0123456789abcdef0123456789abcdef")
	iv := []byte("0123456789abcdef")
	padded := make([]byte, encryptedSize(int64(len(plain))))
	copy(padded, plain)
	block, err := stdaes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(padded, padded)

	getReader := func(_ context.Context, start, end int64) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(padded[start : end+1])), nil
	}

	for _, test := range []struct {
		name       string
		start, end int64
	}{
		{name: "full", start: 0, end: int64(len(plain) - 1)},
		{name: "unaligned middle", start: 137, end: 1789},
		{name: "tail", start: int64(len(plain) - 91), end: int64(len(plain) - 1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			reader, err := New().Open(context.Background(), &utils.RangeHeader{Start: test.start, End: test.end}, int64(len(plain)), base64.StdEncoding.EncodeToString(key), base64.StdEncoding.EncodeToString(iv), getReader)
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			got, err := io.ReadAll(reader)
			if err != nil {
				t.Fatal(err)
			}
			want := plain[test.start : test.end+1]
			if !bytes.Equal(got, want) {
				t.Fatalf("decrypted range mismatch: got %d bytes, want %d", len(got), len(want))
			}
		})
	}
}

func TestDecryptReaderReturnsWhenEncryptedSourceEndsEarly(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	iv := []byte("0123456789abcdef")
	encrypted := make([]byte, BlockSize)
	getReader := func(_ context.Context, _, _ int64) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(encrypted)), nil
	}
	reader, err := newDecryptReader(context.Background(), getReader, key, iv, 4*BlockSize, 4*BlockSize, -1)
	if err != nil {
		t.Fatalf("newDecryptReader() error = %v", err)
	}
	defer reader.Close()

	type readResult struct {
		n   int
		err error
	}
	resultCh := make(chan readResult, 1)
	go func() {
		n, err := reader.Read(make([]byte, 2*BlockSize))
		resultCh <- readResult{n: n, err: err}
	}()

	select {
	case result := <-resultCh:
		if result.n != BlockSize || result.err != nil {
			t.Fatalf("Read() = (%d, %v), want (%d, nil)", result.n, result.err, BlockSize)
		}
	case <-time.After(time.Second):
		t.Fatal("Read() did not return after the encrypted source reached EOF")
	}

	if n, err := reader.Read(make([]byte, BlockSize)); n != 0 || err != io.EOF {
		t.Fatalf("second Read() = (%d, %v), want (0, EOF)", n, err)
	}
}

func TestDecryptReaderReusesReadBuffer(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	iv := []byte("0123456789abcdef")
	encrypted := make([]byte, BlockSize*64*102)
	getReader := func(_ context.Context, _, _ int64) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(encrypted)), nil
	}
	reader, err := newDecryptReader(context.Background(), getReader, key, iv, int64(len(encrypted)), int64(len(encrypted)), -1)
	if err != nil {
		t.Fatalf("newDecryptReader() error = %v", err)
	}
	defer reader.Close()

	buf := make([]byte, BlockSize*64)
	if n, err := reader.Read(buf); n != len(buf) || err != nil {
		t.Fatalf("warm-up Read() = (%d, %v), want (%d, nil)", n, err, len(buf))
	}
	if allocs := testing.AllocsPerRun(100, func() {
		if n, err := reader.Read(buf); n != len(buf) || err != nil {
			panic("unexpected decrypt reader result")
		}
	}); allocs != 0 {
		t.Fatalf("Read() allocations = %.2f, want 0", allocs)
	}
}
