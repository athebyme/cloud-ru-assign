package http

import (
	"encoding/json"
	"github.com/athebyme/cloud-ru-assign/internal/core/domain/ratelimit"
	"github.com/athebyme/cloud-ru-assign/internal/core/ports"
	"net/http"
)

// RateLimitAPIHandler обрабатывает CRUD API для rate limiting
type RateLimitAPIHandler struct {
	service ports.RateLimitService
	logger  ports.Logger
}

func NewRateLimitAPIHandler(service ports.RateLimitService, logger ports.Logger) *RateLimitAPIHandler {
	return &RateLimitAPIHandler{
		service: service,
		logger:  logger.With("handler", "RateLimitAPI"),
	}
}

func (h *RateLimitAPIHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/clients", h.handleClients)
	mux.HandleFunc("/clients/", h.handleClient)
}

func (h *RateLimitAPIHandler) handleClients(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listClients(w, r)
	case http.MethodPost:
		h.createClient(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *RateLimitAPIHandler) handleClient(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	clientID := r.URL.Path[len("/api/v1/ratelimit/clients/"):]
	if clientID == "" {
		http.Error(w, "clientID required", http.StatusBadRequest)
		return
	}

	err := h.service.RemoveClient(clientID)
	if err != nil {
		h.respondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// listClients возвращает список всех клиентов
func (h *RateLimitAPIHandler) listClients(w http.ResponseWriter, r *http.Request) {
	clients, err := h.service.ListClients()
	if err != nil {
		h.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.respondWithJSON(w, http.StatusOK, clients)
}

// createClient создает или обновляет клиента
func (h *RateLimitAPIHandler) createClient(w http.ResponseWriter, r *http.Request) {
	var settings ratelimit.RateLimitSettings
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	if err := h.service.CreateOrUpdateClient(&settings); err != nil {
		h.respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.respondWithJSON(w, http.StatusCreated, settings)
}

// respondWithJSON отправляет JSON ответ
func (h *RateLimitAPIHandler) respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, _ := json.Marshal(payload)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(response)
}

// respondWithError отправляет ошибку в JSON формате
func (h *RateLimitAPIHandler) respondWithError(w http.ResponseWriter, code int, message string) {
	h.respondWithJSON(w, code, map[string]string{"error": message})
}
