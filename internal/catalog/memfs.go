package catalog

import (
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"
)

// memFS is a small read-only in-memory fs.FS keyed by slash paths.
type memFS struct{ files map[string][]byte }

func (m memFS) isDir(name string) bool {
	if name == "." {
		return true
	}
	for p := range m.files {
		if strings.HasPrefix(p, name+"/") {
			return true
		}
	}
	return false
}

func (m memFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	if data, ok := m.files[name]; ok {
		return &memFile{info: memInfo{name: path.Base(name), size: int64(len(data))}, data: data}, nil
	}
	if !m.isDir(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	entries, _ := m.ReadDir(name)
	return &memDir{info: memInfo{name: path.Base(name), dir: true}, entries: entries}, nil
}

func (m memFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if !fs.ValidPath(name) || !m.isDir(name) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}
	prefix := ""
	if name != "." {
		prefix = name + "/"
	}
	seen := map[string]fs.DirEntry{}
	for p, data := range m.files {
		rest, ok := strings.CutPrefix(p, prefix)
		if !ok {
			continue
		}
		first, _, nested := strings.Cut(rest, "/")
		if nested {
			seen[first] = fs.FileInfoToDirEntry(memInfo{name: first, dir: true})
		} else {
			seen[first] = fs.FileInfoToDirEntry(memInfo{name: first, size: int64(len(data))})
		}
	}
	out := make([]fs.DirEntry, 0, len(seen))
	for _, e := range seen {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out, nil
}

type memInfo struct {
	name string
	size int64
	dir  bool
}

func (i memInfo) Name() string { return i.name }
func (i memInfo) Size() int64  { return i.size }
func (i memInfo) Mode() fs.FileMode {
	if i.dir {
		return fs.ModeDir | 0o555
	}
	return 0o444
}
func (i memInfo) ModTime() time.Time { return time.Time{} }
func (i memInfo) IsDir() bool        { return i.dir }
func (i memInfo) Sys() any           { return nil }

type memFile struct {
	info memInfo
	data []byte
	off  int
}

func (f *memFile) Stat() (fs.FileInfo, error) { return f.info, nil }
func (f *memFile) Close() error               { return nil }
func (f *memFile) Read(b []byte) (int, error) {
	if f.off >= len(f.data) {
		return 0, io.EOF
	}
	n := copy(b, f.data[f.off:])
	f.off += n
	return n, nil
}

type memDir struct {
	info    memInfo
	entries []fs.DirEntry
}

func (d *memDir) Stat() (fs.FileInfo, error) { return d.info, nil }
func (d *memDir) Close() error               { return nil }
func (d *memDir) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: d.info.name, Err: fs.ErrInvalid}
}
func (d *memDir) ReadDir(n int) ([]fs.DirEntry, error) {
	if n <= 0 {
		out := d.entries
		d.entries = nil
		return out, nil
	}
	if len(d.entries) == 0 {
		return nil, io.EOF
	}
	if n > len(d.entries) {
		n = len(d.entries)
	}
	out := d.entries[:n]
	d.entries = d.entries[n:]
	return out, nil
}
