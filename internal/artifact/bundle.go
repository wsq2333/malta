package artifact

import (
	"encoding/json"
	"github.com/wsq2333/malta/internal/domain"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

const MaxBytes = 32 << 20

type File struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Data   []byte `json:"data"`
}
type Bundle struct {
	SchemaVersion string     `json:"schema_version"`
	Run           domain.Run `json:"run"`
	Files         []File     `json:"files"`
}

func (b Bundle) Validate() error {
	if b.SchemaVersion != domain.Schema {
		return domain.E("invalid", "unsupported bundle schema")
	}
	if err := b.Run.Validate(); err != nil {
		return err
	}
	seen := map[string]bool{}
	total := 0
	for _, f := range b.Files {
		total += len(f.Data)
		if !fs.ValidPath(f.Path) || strings.Contains(f.Path, "\\") || f.Path == "bundle.json" || seen[f.Path] || domain.BytesHash(f.Data) != f.SHA256 || total > MaxBytes {
			return domain.E("invalid", "invalid artifact path, duplicate, checksum or bundle size")
		}
		seen[f.Path] = true
	}
	return nil
}
func Collect(dir string, r domain.Run) (Bundle, error) {
	b := Bundle{SchemaVersion: domain.Schema, Run: r, Files: []File{}}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return domain.E("invalid", "symlink artifact rejected")
		}
		rel, e := filepath.Rel(dir, p)
		if e != nil {
			return e
		}
		if rel == "bundle.json" {
			return nil
		}
		f, e := os.Open(p)
		if e != nil {
			return e
		}
		data, e := io.ReadAll(io.LimitReader(f, MaxBytes+1))
		f.Close()
		if e != nil {
			return e
		}
		b.Files = append(b.Files, File{filepath.ToSlash(rel), domain.BytesHash(data), data})
		return nil
	})
	if err != nil {
		return b, err
	}
	return b, b.Validate()
}
func Read(dir string) (Bundle, error) {
	var b Bundle
	f, e := os.Open(filepath.Join(dir, "bundle.json"))
	if e != nil {
		return b, e
	}
	defer f.Close()
	e = domain.DecodeJSON(io.LimitReader(f, MaxBytes*2), &b)
	if e != nil {
		return b, e
	}
	return b, b.Validate()
}
func Write(dir string, b Bundle) error {
	if e := b.Validate(); e != nil {
		return e
	}
	if _, e := os.Lstat(dir); e == nil {
		return domain.E("conflict", "bundle destination already exists")
	}
	if e := os.MkdirAll(filepath.Dir(dir), 0755); e != nil {
		return e
	}
	tmp, e := os.MkdirTemp(filepath.Dir(dir), ".bundle-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(tmp)
	for _, f := range b.Files {
		p := filepath.Join(tmp, filepath.FromSlash(f.Path))
		if e = os.MkdirAll(filepath.Dir(p), 0755); e != nil {
			return e
		}
		if e = os.WriteFile(p, f.Data, 0644); e != nil {
			return e
		}
	}
	if e = domain.Write(filepath.Join(tmp, "bundle.json"), b); e != nil {
		return e
	}
	return os.Rename(tmp, dir)
}
func Digest(b Bundle) string {
	copyB := b
	copyB.Files = append([]File{}, b.Files...)
	sort.Slice(copyB.Files, func(i, j int) bool { return copyB.Files[i].Path < copyB.Files[j].Path })
	return domain.Hash(copyB)
}
func Get(b Bundle, name string) ([]byte, error) {
	if path.Clean(name) != name {
		return nil, domain.E("invalid", "invalid artifact path")
	}
	for _, f := range b.Files {
		if f.Path == name {
			return f.Data, nil
		}
	}
	return nil, domain.E("not_found", "artifact not found")
}
func Decode(data []byte) (Bundle, error) {
	var b Bundle
	e := json.Unmarshal(data, &b)
	if e != nil {
		return b, e
	}
	return b, b.Validate()
}
