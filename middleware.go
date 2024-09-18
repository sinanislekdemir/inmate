package main

import (
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

func AuthMiddleware(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if the request has the correct token
		if c.GetHeader("Authorization") != "Token "+token {
			c.JSON(401, gin.H{"error": "Unauthorized"})
			c.Abort()
			return
		}
		c.Next()
	}
}

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
		logrus.WithFields(logrus.Fields{
			"method":   c.Request.Method,
			"url":      sanitizedURL,
			"clientIP": c.ClientIP(),
			"latency":  latency,
		}).Info("Request processed")
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
