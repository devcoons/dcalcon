package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type File struct {
	URL      string `json:"url"`
	Session  string `json:"session,omitempty"`
	Username string `json:"username,omitempty"`
}

func Path() string {
	if p := strings.TrimSpace(os.Getenv("DCALCON_CLI_CONFIG")); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		dir = "."
	}
	return filepath.Join(dir, "dcalcon", "cli.json")
}

func Load(path string) (*File, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &File{URL: defaultURL()}, nil
		}
		return nil, err
	}
	var f File
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, err
	}
	if strings.TrimSpace(f.URL) == "" {
		f.URL = defaultURL()
	}
	return &f, nil
}

func (f *File) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(path, raw, 0o600)
}

func defaultURL() string {
	if u := strings.TrimSpace(os.Getenv("DCALCON_URL")); u != "" {
		return strings.TrimRight(u, "/")
	}
	return "http://127.0.0.1:8080"
}
