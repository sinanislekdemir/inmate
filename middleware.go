package main

import (
	"log"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
)

// MaskedLogger will mask sensitive query string parameters
func MaskedLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Start time
		start := time.Now()

		// Process request
		c.Next()

		// End time
		end := time.Now()
		latency := end.Sub(start)

		// Sanitize query string by masking sensitive parameters
		sanitizedURL := maskQueryParams(c.Request.URL)

		// Log the sanitized request
		log.Printf("GIN: [%s] %s %s in %v", c.Request.Method, sanitizedURL, c.ClientIP(), latency)
	}
}

// maskQueryParams masks sensitive parameters in the query string
func maskQueryParams(u *url.URL) string {
	// List of sensitive parameters
	sensitiveParams := []string{"u", "token", "p"}

	query := u.Query()

	// Mask sensitive parameters
	for _, param := range sensitiveParams {
		if query.Has(param) {
			query.Set(param, "hidden")
		}
	}

	// Build the sanitized URL
	u.RawQuery = query.Encode()
	return u.String()
}
