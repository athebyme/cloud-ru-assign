package domain

import "net/url"

// Backend представляет основную доменную сущность бэкенд-сервера
// содержит только URL, статус управляется в других слоях (например, репозитории)
type Backend struct {
	URL *url.URL
}
