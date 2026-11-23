package fact

import (
	"bytes"
	"io"
	"io/fs"
	"time"
)

// overlayFS is a file system with some files' contents replaced. It adds
// and removes no file.
type overlayFS struct {
	base  fs.FS
	files map[string][]byte
}

func (o overlayFS) Open(name string) (fs.File, error) {
	data, ok := o.files[name]
	if !ok {
		return o.base.Open(name)
	}
	return &memFile{name: name, r: bytes.NewReader(data), size: int64(len(data))}, nil
}

func (o overlayFS) ReadFile(name string) ([]byte, error) {
	if data, ok := o.files[name]; ok {
		return append([]byte(nil), data...), nil
	}
	return fs.ReadFile(o.base, name)
}

func (o overlayFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return fs.ReadDir(o.base, name)
}

type memFile struct {
	name string
	r    *bytes.Reader
	size int64
}

func (f *memFile) Stat() (fs.FileInfo, error) { return memInfo{name: f.name, size: f.size}, nil }
func (f *memFile) Read(b []byte) (int, error) { return f.r.Read(b) }
func (f *memFile) Close() error               { return nil }

var _ io.Reader = (*memFile)(nil)

type memInfo struct {
	name string
	size int64
}

func (i memInfo) Name() string {
	for j := len(i.name) - 1; j >= 0; j-- {
		if i.name[j] == '/' {
			return i.name[j+1:]
		}
	}
	return i.name
}
func (i memInfo) Size() int64        { return i.size }
func (i memInfo) Mode() fs.FileMode  { return 0o444 }
func (i memInfo) ModTime() time.Time { return time.Time{} }
func (i memInfo) IsDir() bool        { return false }
func (i memInfo) Sys() any           { return nil }
