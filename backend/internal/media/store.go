package media

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Asset struct {
	SHA256     string
	MIMEType   string
	Size       int64
	ObjectPath string
}
type Store struct{ Root string }

func (s Store) Put(r io.Reader, mimeType string) (Asset, error) {
	return s.put(r, mimeType, 0)
}

func (s Store) PutLimited(r io.Reader, mimeType string, maxSize int64) (Asset, error) {
	return s.put(r, mimeType, maxSize)
}

func (s Store) put(r io.Reader, mimeType string, maxSize int64) (Asset, error) {
	if s.Root == "" {
		return Asset{}, fmt.Errorf("object store root is required")
	}
	if err := os.MkdirAll(s.Root, 0o750); err != nil {
		return Asset{}, err
	}
	tmp, err := os.CreateTemp(s.Root, ".media-*")
	if err != nil {
		return Asset{}, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	hash := sha256.New()
	var source io.Reader = r
	if maxSize > 0 {
		source = io.LimitReader(r, maxSize+1)
	}
	size, copyErr := io.Copy(io.MultiWriter(tmp, hash), source)
	closeErr := tmp.Close()
	if copyErr != nil {
		return Asset{}, copyErr
	}
	if closeErr != nil {
		return Asset{}, closeErr
	}
	if maxSize > 0 && size > maxSize {
		return Asset{}, fmt.Errorf("media object exceeds limit of %d bytes", maxSize)
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	relative := filepath.Join(digest[:2], digest[2:4], digest)
	destination := filepath.Join(s.Root, relative)
	if err := os.MkdirAll(filepath.Dir(destination), 0o750); err != nil {
		return Asset{}, err
	}
	if _, err := os.Stat(destination); os.IsNotExist(err) {
		if err := os.Rename(tmpPath, destination); err != nil {
			return Asset{}, err
		}
	}
	return Asset{SHA256: digest, MIMEType: mimeType, Size: size, ObjectPath: relative}, nil
}
