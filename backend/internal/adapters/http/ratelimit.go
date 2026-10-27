package http

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"golang.org/x/time/rate"
)

// Per-client limits, enforced in memory (so per API replica, not across
// replicas -- a shared limit would need Redis, not warranted at this scale).
//
// The global limit is a generous ceiling against a runaway client or a scrape;
// a person using the app never gets near it. The strict one covers routes
// where a request is expensive or guessable: credential and code endpoints
// (brute force, email-sending abuse) and the two routes that call a paid LLM.
const (
	globalRatePerSecond = rate.Limit(20)
	globalBurst         = 40
	strictRatePerSecond = rate.Limit(0.2) // 12 per minute sustained
	strictBurst         = 10
)

func rateLimiter(perSecond rate.Limit, burst int) echo.MiddlewareFunc {
	store := middleware.NewRateLimiterMemoryStoreWithConfig(middleware.RateLimiterMemoryStoreConfig{
		Rate:      perSecond,
		Burst:     burst,
		ExpiresIn: 3 * time.Minute, // idle clients are forgotten, bounding memory
	})
	retryAfter := strconv.Itoa(int(1/float64(perSecond)) + 1)
	return middleware.RateLimiterWithConfig(middleware.RateLimiterConfig{
		Store: store,
		// RealIP honors X-Forwarded-For only when the immediate peer is a
		// private/loopback address (the reverse proxy); a client connecting
		// directly cannot spoof its way out of the limit -- see
		// echo.ExtractIPFromXFFHeader, set on the Server in NewServer.
		IdentifierExtractor: func(c echo.Context) (string, error) { return c.RealIP(), nil },
		DenyHandler: func(c echo.Context, identifier string, _ error) error {
			// Without this a brute-force attempt that got throttled leaves no trace.
			log.Printf("rate limited %s %s %s", identifier, c.Request().Method, c.Request().URL.Path)
			c.Response().Header().Set("Retry-After", retryAfter)
			return c.JSON(http.StatusTooManyRequests, echo.Map{"error": "too many requests, slow down"})
		},
	})
}
