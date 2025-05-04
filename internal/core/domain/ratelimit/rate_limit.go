package ratelimit

type RateLimitSettings struct {
	ClientID      string `json:"client_id"`
	Capacity      int64  `json:"capacity"`
	RatePerSecond int64  `json:"rate_per_second"`
}
