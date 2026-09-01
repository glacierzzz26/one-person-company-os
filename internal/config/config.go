package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	DBPath    string     `yaml:"db_path"`
	Providers []Provider `yaml:"providers"`
}

// Provider 是 config 中声明的 Model Provider。Phase 2 仅声明/展示,不真实调用;
// api_key_env 指示 API key 所在环境变量名(真实接入时注入)。
type Provider struct {
	Name      string `yaml:"name"`
	Type      string `yaml:"type"`
	Model     string `yaml:"model"`
	Endpoint  string `yaml:"endpoint"`
	APIKeyEnv string `yaml:"api_key_env"`
}

func Load(path string) (Config, error) {
	cfg := Config{DBPath: "./os.db"}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read config %q: %w", path, err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config %q: %w", path, err)
	}
	return cfg, nil
}
