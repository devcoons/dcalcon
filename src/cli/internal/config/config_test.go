package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dcalcon", "cli.json")
	f := &File{URL: "http://127.0.0.1:8080", Session: "abc", Username: "admin"}
	if err := f.Save(path); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %o", st.Mode().Perm())
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.URL != f.URL || got.Session != f.Session || got.Username != f.Username {
		t.Fatalf("%+v", got)
	}
}

func TestLoadMissingUsesDefaultURL(t *testing.T) {
	t.Setenv("DCALCON_URL", "")
	got, err := Load(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got.URL != "http://127.0.0.1:8080" {
		t.Fatalf("url %s", got.URL)
	}
}
