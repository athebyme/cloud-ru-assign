package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"strings" // For level conversion
	"time"
)

type HealthCheckConfig struct {
	Enabled  bool          `yaml:"enabled"`
	Interval time.Duration `yaml:"interval"`
	Timeout  time.Duration `yaml:"timeout"`
	Path     string        `yaml:"path"`
}

type LogConfig struct {
	Level  string `yaml:"level"`  // debug, info, warn, error
	Format string `yaml:"format"` // json, text
}

type Config struct {
	ListenAddress string            `yaml:"listenAddress"`
	Backends      []string          `yaml:"backends"`
	Log           LogConfig         `yaml:"log"`
	HealthCheck   HealthCheckConfig `yaml:"healthCheck"`
}

func LoadConfig(configPath string) (*Config, error) {
	conf := &Config{
		ListenAddress: ":8080",
		Log:           LogConfig{Level: "info", Format: "text"},
		HealthCheck: HealthCheckConfig{
			Enabled:  true,
			Interval: 15 * time.Second,
			Timeout:  3 * time.Second,
		},
	}
	yamlFile, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения файла конфигурации %s: %w", configPath, err)
	}

	err = yaml.Unmarshal(yamlFile, conf)
	if err != nil {
		return nil, fmt.Errorf("ошибка парсинга YAML %s: %w", configPath, err)
	}

	// нормализуем значения конфига логов
	conf.Log.Level = strings.ToLower(conf.Log.Level)
	conf.Log.Format = strings.ToLower(conf.Log.Format)
	if conf.Log.Level == "" {
		conf.Log.Level = "info"
	}
	if conf.Log.Format == "" {
		conf.Log.Format = "text"
	}

	// валидация обязательных полей
	if len(conf.Backends) == 0 {
		return nil, fmt.Errorf("в конфигурации %s не указаны бэкенды ('backends')", configPath)
	}
	if conf.ListenAddress == "" {
		return nil, fmt.Errorf("в конфигурации %s не указан адрес для прослушивания ('listenAddress')", configPath)
	}

	// проверка на дубликаты бэкендов
	seen := make(map[string]bool)
	var uniqueBackends []string
	for _, backend := range conf.Backends {
		if !seen[backend] {
			seen[backend] = true
			uniqueBackends = append(uniqueBackends, backend)
		} else {
			// ошибка при обнаружении дубликата, в теории как бы можно еще просто "всхлопывать"
			// дубликаты в 1, но оставлю проброс ошибки выше
			return nil, fmt.Errorf("обнаружен дублирующийся адрес бэкенда в конфигурации: %s", backend)
		}
	}
	conf.Backends = uniqueBackends

	// валидация конфигурации health check'ов
	if conf.HealthCheck.Enabled {
		if conf.HealthCheck.Interval <= 0 {
			return nil, fmt.Errorf("healthCheck.interval должен быть положительным значением")
		}
		if conf.HealthCheck.Timeout <= 0 {
			return nil, fmt.Errorf("healthCheck.timeout должен быть положительным значением")
		}
		if conf.HealthCheck.Timeout >= conf.HealthCheck.Interval {
			// просто предупреждаем, если таймаут слишком большой
			// тут пришла альтернативная идея давать выброс ошибки наверх
			fmt.Printf("Предупреждение: healthCheck.timeout (%v) близок или больше healthCheck.interval (%v)\n", conf.HealthCheck.Timeout, conf.HealthCheck.Interval)
		}
	}

	return conf, nil
}
