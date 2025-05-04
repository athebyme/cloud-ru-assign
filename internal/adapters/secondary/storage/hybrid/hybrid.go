package hybrid

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/athebyme/cloud-ru-assign/internal/core/domain/ratelimit"
	"github.com/athebyme/cloud-ru-assign/internal/core/ports"
	"github.com/go-redis/redis/v8"
	"time"
)

// HybridRateLimiter использует:
// - Redis для хранения текущих токенов (быстрый доступ)
// - PostgreSQL для хранения настроек (персистентность)
type HybridRateLimiter struct {
	db     *sql.DB
	redis  *redis.Client
	logger ports.Logger
}

var _ ports.RateLimiter = (*HybridRateLimiter)(nil)

func NewHybridRateLimiter(pgConnStr string, redisAddr string, logger ports.Logger) (*HybridRateLimiter, error) {
	db, err := sql.Open("postgres", pgConnStr)
	if err != nil {
		return nil, fmt.Errorf("не удалось открыть соединение postgres: %w", err)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr: redisAddr,
		DB:   0,
	})

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("не удалось пингануть postgres: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		db.Close()
		rdb.Close()
		return nil, fmt.Errorf("не удалось пингануть redis: %w", err)
	}

	if err := createSchema(db); err != nil {
		db.Close()
		rdb.Close()
		return nil, fmt.Errorf("не удалось создать схему базы данных: %w", err)
	}

	return &HybridRateLimiter{
		db:     db,
		redis:  rdb,
		logger: logger.With("component", "HybridRateLimiter"),
	}, nil
}

func (h *HybridRateLimiter) RemoveRateLimit(clientID string) error {
	ctx := context.Background()
	var combinedErr error

	result, err := h.db.ExecContext(ctx, `DELETE FROM rate_limit_settings WHERE client_id = $1`, clientID)
	if err != nil {
		h.logger.Error("Не удалось удалить настройки лимита скорости из PostgreSQL", "clientID", clientID, "error", err)
		combinedErr = fmt.Errorf("не удалось удалить настройки из БД: %w", err)
	} else {
		rowsAffected, _ := result.RowsAffected()
		if rowsAffected > 0 {
			h.logger.Info("Настройки лимита скорости удалены из PostgreSQL", "clientID", clientID, "rowsAffected", rowsAffected)
		} else {
			h.logger.Info("Настройки лимита скорости для удаления не найдены в PostgreSQL", "clientID", clientID)
		}
	}

	keysToDelete := []string{
		"ratelimit:settings:" + clientID,
		"ratelimit:" + clientID + ":tokens",
		"ratelimit:" + clientID + ":last_refill",
	}
	deletedCount, redisErr := h.redis.Del(ctx, keysToDelete...).Result()
	if redisErr != nil {
		h.logger.Error("Не удалось удалить ключи лимита скорости из Redis", "clientID", clientID, "keys", keysToDelete, "error", redisErr)
		if combinedErr != nil {
			combinedErr = fmt.Errorf("%w; не удалось удалить ключи из Redis: %w", combinedErr, redisErr)
		} else {
			combinedErr = fmt.Errorf("не удалось удалить ключи из Redis: %w", redisErr)
		}
	} else {
		h.logger.Info("Ключи лимита скорости удалены из Redis", "clientID", clientID, "keys", keysToDelete, "count", deletedCount)
	}

	return combinedErr
}

// Allow использует Redis Lua script для атомарной проверки токенов
func (h *HybridRateLimiter) Allow(clientID string) bool {
	ctx := context.Background()

	settings, err := h.getSettings(clientID)
	if err != nil {
		return true
	}

	script := `
		local key = KEYS[1]
		local capacity = tonumber(ARGV[1])
		local rate = tonumber(ARGV[2])
		local now = tonumber(ARGV[3])
		
		local tokens_key = key .. ":tokens"
		local last_refill_key = key .. ":last_refill"
		
		local tokens = redis.call('GET', tokens_key)
		local last_refill = redis.call('GET', last_refill_key)
		
		if not tokens then
			tokens = capacity
			last_refill = now
		else
			tokens = tonumber(tokens)
			last_refill = tonumber(last_refill)
		end
		
		-- Пополняем токены
		local elapsed = now - last_refill
		local tokens_to_add = math.floor(elapsed * rate)
		tokens = math.min(capacity, tokens + tokens_to_add)
		
		-- Проверяем и расходуем токен
		if tokens > 0 then
			tokens = tokens - 1
			redis.call('SET', tokens_key, tokens)
			redis.call('SET', last_refill_key, now)
			return 1
		else
			return 0
		end
	`

	result, err := h.redis.Eval(ctx, script, []string{"ratelimit:" + clientID},
		settings.Capacity, settings.RatePerSecond, time.Now().Unix()).Result()

	if err != nil {
		h.logger.Error("Redis eval failed", "error", err)
		return true
	}

	return result.(int64) == 1
}

// SetRateLimit сохраняет настройки в PostgreSQL
func (h *HybridRateLimiter) SetRateLimit(clientID string, settings *ratelimit.RateLimitSettings) error {
	ctx := context.Background()

	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return err
	}

	_, err = h.db.ExecContext(ctx, `
		INSERT INTO rate_limit_settings (client_id, settings)
		VALUES ($1, $2)
		ON CONFLICT (client_id) DO UPDATE
		SET settings = $2, updated_at = CURRENT_TIMESTAMP
	`, clientID, settingsJSON)

	if err != nil {
		return err
	}

	h.redis.Del(ctx, "ratelimit:settings:"+clientID)

	return nil
}

// getSettings получает настройки (с кешированием в Redis)
func (h *HybridRateLimiter) getSettings(clientID string) (*ratelimit.RateLimitSettings, error) {
	ctx := context.Background()
	settingsCacheKey := "ratelimit:settings:" + clientID

	cached, err := h.redis.Get(ctx, settingsCacheKey).Result()
	if err == nil {
		var settings ratelimit.RateLimitSettings
		if err := json.Unmarshal([]byte(cached), &settings); err == nil {
			return &settings, nil
		}
	}

	var settingsJSON []byte
	err = h.db.QueryRowContext(ctx, `
		SELECT settings FROM rate_limit_settings WHERE client_id = $1
	`, clientID).Scan(&settingsJSON)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	var settings ratelimit.RateLimitSettings
	if err := json.Unmarshal(settingsJSON, &settings); err != nil {
		return nil, err
	}

	h.redis.Set(ctx, settingsCacheKey, settingsJSON, time.Hour)

	return &settings, nil
}

// Stop закрывает соединения
func (h *HybridRateLimiter) Stop() {
	h.db.Close()
	h.redis.Close()
}

func createSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS rate_limit_settings (
			client_id VARCHAR(255) PRIMARY KEY,
			settings JSONB NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		);
		
		CREATE INDEX IF NOT EXISTS idx_rate_limit_settings_client_id ON rate_limit_settings(client_id);
	`)
	return err
}
