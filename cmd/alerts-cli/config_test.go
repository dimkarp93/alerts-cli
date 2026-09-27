package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigPathFollowsXDG(t *testing.T) {
	t.Setenv("HOME", "/h")
	t.Setenv("XDG_CONFIG_HOME", "/x")
	got, err := configPath()
	if err != nil {
		t.Fatal(err)
	}
	if got != "/x/alerts-cli/config.json" {
		t.Fatalf("got %q", got)
	}
}

func TestConfigPathFallsBackToTheLegacyFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))

	legacy := filepath.Join(home, ".config", "alerts-cli", "config.json")
	newPath := filepath.Join(home, "xdg", "alerts-cli", "config.json")

	got, err := configPath()
	if err != nil || got != newPath {
		t.Fatalf("nothing exists: got %q, %v", got, err)
	}

	if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err = configPath()
	if err != nil || got != legacy {
		t.Fatalf("legacy exists: got %q, %v", got, err)
	}
}

func TestPathEntries(t *testing.T) {
	t.Setenv("HOME", "/h")
	t.Setenv("XDG_CONFIG_HOME", "/x")
	entries := pathEntries()
	if len(entries) != 1 || entries[0].Name != "config" || entries[0].Path != "/x/alerts-cli/config.json" {
		t.Fatalf("got %+v", entries)
	}
}
