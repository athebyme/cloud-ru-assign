package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/athebyme/cloud-ru-assign/internal/core/domain/ratelimit"
	"github.com/athebyme/cloud-ru-assign/internal/core/ports"
	"time"
)

// PostgresRateLimiter реализует персистентный rate limiter через PostgreSQL
type PostgresRateLimiter struct {
	db     *sql.DB
	logger ports.Logger
}

// NewPostgresRateLimiter создает новый PostgreSQL rate limiter
func NewPostgresRateLimiter(connStr string, logger ports.Logger) (*PostgresRateLimiter, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	limiter := &PostgresRateLimiter{
		db:     db,
		logger: logger.With("component", "PostgresRateLimiter"),
	}

	if err := limiter.initSchema(); err != nil {
		return nil, err
	}

	return limiter, nil
}

func (r *PostgresRateLimiter) initSchema() error {
	ctx := context.Background()
	schema := `
		CREATE TABLE IF NOT EXISTS rate_limits (
			client_id VARCHAR(255) PRIMARY KEY,
			capacity BIGINT NOT NULL,
			rate_per_second BIGINT NOT NULL,
			tokens BIGINT NOT NULL,
			last_refill TIMESTAMP WITH TIME ZONE NOT NULL
		);
		
		CREATE INDEX IF NOT EXISTS idx_rate_limits_client_id ON rate_limits(client_id);
	`
	_, err := r.db.ExecContext(ctx, schema)
	return err
}

// Allow проверяет, может ли клиент сделать запрос, используя transaction
func (r *PostgresRateLimiter) Allow(clientID string) bool {
	ctx := context.Background()

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		r.logger.Error("Failed to begin transaction", "error", err)
		return true // по умолчанию разрешаем при ошибке
	}
	defer tx.Rollback()

	var capacity, ratePerSecond, tokens int64
	var lastRefill time.Time

	err = tx.QueryRowContext(ctx, `
		SELECT capacity, rate_per_second, tokens, last_refill 
		FROM rate_limits 
		WHERE client_id = $1 
		FOR UPDATE
	`, clientID).Scan(&capacity, &ratePerSecond, &tokens, &lastRefill)

	if errors.Is(err, sql.ErrNoRows) {
		return true // нет ограничения для клиента
	} else if err != nil {
		r.logger.Error("Failed to get rate limit", "error", err)
		return true
	}

	now := time.Now()
	elapsed := now.Sub(lastRefill).Seconds()
	tokensToAdd := int64(elapsed * float64(ratePerSecond))
	newTokens := min(capacity, tokens+tokensToAdd)

	// можно ли взять токен
	if newTokens > 0 {
		newTokens--
		_, err = tx.ExecContext(ctx, `
			UPDATE rate_limits 
			SET tokens = $1, last_refill = $2 
			WHERE client_id = $3
		`, newTokens, now, clientID)

		if err != nil {
			r.logger.Error("Failed to update rate limit", "error", err)
			return true
		}

		if err := tx.Commit(); err != nil {
			r.logger.Error("Failed to commit transaction", "error", err)
			return true
		}

		r.logger.Debug("Token consumed", "client", clientID, "tokens_left", newTokens)
		return true
	}

	r.logger.Debug("Rate limit exceeded", "client", clientID)
	return false
}

// SetRateLimit устанавливает или обновляет ограничения для клиента
func (r *PostgresRateLimiter) SetRateLimit(clientID string, settings *ratelimit.RateLimitSettings) error {
	ctx := context.Background()

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO rate_limits (client_id, capacity, rate_per_second, tokens, last_refill)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (client_id) DO UPDATE
		SET capacity = $2, rate_per_second = $3, tokens = $4, last_refill = $5
	`, clientID, settings.Capacity, settings.RatePerSecond, settings.Capacity, time.Now())

	if err != nil {
		return fmt.Errorf("failed to set rate limit: %w", err)
	}

	r.logger.Info("Rate limit set", "client", clientID,
		"capacity", settings.Capacity, "rate", settings.RatePerSecond)
	return nil
}

// RemoveRateLimit удаляет ограничения для клиента
func (r *PostgresRateLimiter) RemoveRateLimit(clientID string) {
	ctx := context.Background()

	_, err := r.db.ExecContext(ctx, "DELETE FROM rate_limits WHERE client_id = $1", clientID)
	if err != nil {
		r.logger.Error("Failed to remove rate limit", "error", err, "client", clientID)
	} else {
		r.logger.Info("Rate limit removed", "client", clientID)
	}
}

func (r *PostgresRateLimiter) Stop() {
	if err := r.db.Close(); err != nil {
		r.logger.Error("Failed to close database connection", "error", err)
	}
}

func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
