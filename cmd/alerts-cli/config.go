package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dimkarp93/install-libs/xdgpath"
)

type config struct {
	VM string `json:"vm"`
	AM string `json:"am"`
}

func configPath() (string, error) {
	primary := filepath.Join(xdgpath.ConfigDir("alerts-cli"), "config.json")
	home, err := os.UserHomeDir()
	if err != nil {
		return primary, nil
	}
	legacy := filepath.Join(home, ".config", "alerts-cli", "config.json")
	return xdgpath.WithLegacy(primary, legacy), nil
}

func pathEntries() []xdgpath.Entry {
	path, err := configPath()
	if err != nil {
		return nil
	}
	return []xdgpath.Entry{{Name: "config", Path: path}}
}

func loadConfig() (config, error) {
	path, err := configPath()
	if err != nil {
		return config{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return config{}, fmt.Errorf("не удалось прочитать %s: %w (создайте файл с полями \"vm\" и \"am\" — адресами VictoriaMetrics и Alertmanager)", path, err)
	}
	var c config
	if err := json.Unmarshal(data, &c); err != nil {
		return config{}, fmt.Errorf("%s: %w", path, err)
	}
	if c.VM == "" {
		return config{}, fmt.Errorf("%s: не задано поле \"vm\"", path)
	}
	if c.AM == "" {
		return config{}, fmt.Errorf("%s: не задано поле \"am\"", path)
	}
	return c, nil
}
