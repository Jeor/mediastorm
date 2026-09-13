package importer

import (
	"bytes"
	"io"
	"io/fs"
)

// RAR3/4 use the seven-byte signature; RAR5 has a different version byte.
// Read only the signature; subsequent indexing reuses the filesystem cache.
func isRar3Archive(fsys fs.FS, name string) (bool, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return false, err
	}
	defer f.Close()
	var signature [7]byte
	if _, err := io.ReadFull(f, signature[:]); err != nil {
		return false, err
	}
	return bytes.Equal(signature[:], []byte("Rar!\x1a\x07\x00")), nil
}
