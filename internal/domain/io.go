package domain

import (
	"bytes"
	"encoding/json"
	"gopkg.in/yaml.v3"
	"io"
	"os"
	"path/filepath"
)

func Decode(b []byte, out any) error {
	var raw any
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return E("invalid", err.Error())
	}
	j, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(j))
	d.DisallowUnknownFields()
	if err = d.Decode(out); err != nil {
		return E("invalid", err.Error())
	}
	return nil
}
func Read(path string, out any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(b) > 32<<20 {
		return E("invalid", "input exceeds 32 MiB")
	}
	return Decode(b, out)
}
func Write(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".malta-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(append(b, '\n')); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(name, path)
}
func DecodeJSON(r io.Reader, v any) error {
	d := json.NewDecoder(r)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return E("invalid", err.Error())
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return E("invalid", "expected one JSON document")
	}
	return nil
}
