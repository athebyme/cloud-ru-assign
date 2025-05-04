package hybrid

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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

func NewHybridRateLimiter(pgConnStr string, redisAddr string, logger ports.Logger) (*HybridRateLimiter, error) {
	db, err := sql.Open("postgres", pgConnStr)
	if err != nil {
		return nil, err
	}

	rdb := redis.NewClient(&redis.Options{
		Addr: redisAddr,
		DB:   0,
	})

	if err := db.Ping(); err != nil {
		return nil, err
	}
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		return nil, err
	}

	if err := createSchema(db); err != nil {
		return nil, err
	}

	return &HybridRateLimiter{
		db:     db,
		redis:  rdb,
		logger: logger.With("component", "HybridRateLimiter"),
	}, nil
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
