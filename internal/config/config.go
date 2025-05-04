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

type RateLimitConfig struct {
	Enabled              bool  `yaml:"enabled"`
	Middleware           bool  `yaml:"middleware"`
	DefaultCapacity      int64 `yaml:"defaultCapacity"`
	DefaultRatePerSecond int64 `yaml:"defaultRatePerSecond"`
}

type LoadBalancerConfig struct {
	Strategy string `yaml:"strategy"` // round-robin, least-connections, random
}

type Config struct {
	ListenAddress string             `yaml:"listenAddress"`
	Backends      []string           `yaml:"backends"`
	Log           LogConfig          `yaml:"log"`
	HealthCheck   HealthCheckConfig  `yaml:"healthCheck"`
	RateLimit     RateLimitConfig    `yaml:"rateLimit"`
	LoadBalancer  LoadBalancerConfig `yaml:"loadBalancer"`
}

const (
	StrategyRoundRobin       = "round-robin"
	StrategyLeastConnections = "least-connections"
	StrategyRandom           = "random"
)

func LoadConfig(configPath string) (*Config, error) {
	conf := &Config{
		ListenAddress: ":8080",
		Log:           LogConfig{Level: "info", Format: "text"},
		HealthCheck: HealthCheckConfig{
			Enabled:  true,
			Interval: 15 * time.Second,
			Timeout:  3 * time.Second,
		},
		RateLimit: RateLimitConfig{
			Enabled:              true,
			Middleware:           true,
			DefaultCapacity:      100,
			DefaultRatePerSecond: 10,
		},
		LoadBalancer: LoadBalancerConfig{
			Strategy: StrategyRoundRobin, // значение по умолчанию
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

	// нормализуем стратегию балансировки
	conf.LoadBalancer.Strategy = strings.ToLower(conf.LoadBalancer.Strategy)
	if conf.LoadBalancer.Strategy == "" {
		conf.LoadBalancer.Strategy = StrategyRoundRobin
	}

	// валидация стратегии балансировки
	switch conf.LoadBalancer.Strategy {
	case StrategyRoundRobin, StrategyLeastConnections, StrategyRandom:
		// допустимые стратегии
	default:
		return nil, fmt.Errorf("неподдерживаемая стратегия балансировки: %s. Допустимые значения: %s, %s, %s",
			conf.LoadBalancer.Strategy, StrategyRoundRobin, StrategyLeastConnections, StrategyRandom)
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
			fmt.Printf("Предупреждение: healthCheck.timeout (%v) близок или больше healthCheck.interval (%v)\n",
				conf.HealthCheck.Timeout, conf.HealthCheck.Interval)
		}
	}

	return conf, nil
}
