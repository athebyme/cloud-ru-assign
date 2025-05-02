package main

import (
	"cloud-ru-assign/internal/config"
	"cloud-ru-assign/internal/load_balancer"
	"cloud-ru-assign/internal/server"
	"flag"
	"log"
	"os"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	configPath := flag.String("config", "./configs/config.yml", "Путь к файлу конфигурации YAML")
	flag.Parse()

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Printf("Не удалось загрузить конфигурацию: %v", err)
		os.Exit(1)
	}
	log.Printf("Конфигурация загружена: Адрес=%s, Бэкенды=%v", cfg.ListenAddress, cfg.Backends)

	pool, err := load_balancer.NewServerPool(cfg.Backends)
	if err != nil {
		log.Printf("Не удалось создать пул бэкендов: %v", err)
		os.Exit(1)
	}

	lbHandler := load_balancer.NewLoadBalancerHandler(pool)

	server.Run(cfg.ListenAddress, lbHandler)

	log.Println("Приложение завершило работу.")
}
