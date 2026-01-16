package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	AccessToken    string `json:"accessToken"`
	UserUUID       string `json:"userUuid"`
	PublicUserUUID string `json:"publicUserUuid"`
	UserName       string `json:"userName"`
	Email          string `json:"email"`
	Servings       int    `json:"servings"`
	DefaultList    string `json:"defaultList"`
	Locale         string `json:"locale"`
}

func getConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, ".config", "brings")
}

func getConfigPath() string {
	return filepath.Join(getConfigDir(), "config.json")
}

func loadConfig() Config {
	path := getConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}
	}
	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return Config{}
	}
	return config
}

func saveConfig(config Config) error {
	dir := getConfigDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(getConfigPath(), data, 0o600)
}

func clearConfig() error {
	path := getConfigPath()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func isLoggedIn() bool {
	cfg := loadConfig()
	return cfg.AccessToken != ""
}
