package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
)

type Config struct {
	ListenAddress string   `yaml:"listenAddress"`
	Backends      []string `yaml:"backends"`
}

func LoadConfig(configPath string) (*Config, error) {
	conf := &Config{
		ListenAddress: ":8080",
	}

	yamlFile, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения файла конфигурации %s: %w", configPath, err)
	}

	err = yaml.Unmarshal(yamlFile, conf)
	if err != nil {
		return nil, fmt.Errorf("ошибка парсинга YAML %s: %w", configPath, err)
	}

	if len(conf.Backends) == 0 {
		return nil, fmt.Errorf("в конфигурации %s не указаны бэкенды ('backends')", configPath)
	}
	if conf.ListenAddress == "" {
		return nil, fmt.Errorf("в конфигурации %s не указан адрес для прослушивания ('listenAddress')", configPath)
	}

	seen := make(map[string]bool)
	var uniqueBackends []string
	for _, backend := range conf.Backends {
		if !seen[backend] {
			seen[backend] = true
			uniqueBackends = append(uniqueBackends, backend)
		} else {
			return nil, fmt.Errorf("обнаружен дублирующийся адрес бэкенда в конфигурации: %s", backend)
		}
	}

	return conf, nil
}
