package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type config struct {
	VM string `json:"vm"`
	AM string `json:"am"`
}

func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("определение домашней директории: %w", err)
	}
	return filepath.Join(home, ".config", "alerts-cli", "config.json"), nil
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
