package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

type Payload struct {
	Body        []byte
	URLPath     string
	Method      string
	QueryParams url.Values
	Header      http.Header
}

type InfluxDBInstance struct {
	URL     string
	Token   string
	Channel chan Payload
}

func main() {
	logrus.SetFormatter(&logrus.JSONFormatter{})

	LoadConfig("config.yaml")
	instances := createInstances(config.Addresses)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(MaskedLogger())
	// if config.AuthToken is set, check for the token in the request
	if config.AuthToken != "" {
		router.Use(AuthMiddleware(config.AuthToken))
		logrus.Info("Auth middleware enabled")
	}

	router.POST("/write", handleWrite(instances))
	router.GET("/ping", handleGet(instances))
	router.GET("/health", handleGet(instances))
	router.POST("/query", handleQuery(instances))
	router.GET("/query", handleQuery(instances))

	// V2 Compatibilities, UI is not supported!
	router.POST("/api/v2/write", handleWrite(instances))
	router.POST("/api/v2/bucket", handleMutationGin(instances))
	router.POST("/api/v2/delete", handleMutationGin(instances))

	router.GET("/api/v2/query", handleQuery(instances))
	router.POST("/api/v2/query", handleQuery(instances))

	router.GET("/api/v2/tasks", handleGet(instances))
	router.POST("/api/v2/tasks", handleMutationGin(instances))

	router.GET("/api/v2/tasks/*any", handleGet(instances))
	router.POST("/api/v2/tasks/*any", handleMutationGin(instances))

	router.GET("/api/v2/authorizations", handleFeatureNotSupported)
	router.POST("/api/v2/authorizations", handleFeatureNotSupported)
	router.GET("/api/v2/authorizations/*any", handleFeatureNotSupported)
	router.POST("/api/v2/authorizations/*any", handleFeatureNotSupported)
	router.DELETE("/api/v2/authorizations/*any", handleFeatureNotSupported)
	router.PATCH("/api/v2/authorizations/*any", handleFeatureNotSupported)

	router.GET("/api/v2/orgs", handleGet(instances))
	router.GET("/api/v2/orgs/*any", handleGet(instances))
	router.DELETE("/api/v2/orgs/*any", handleFeatureNotSupported)
	router.GET("/", handleHealthCheck)

	router.NoRoute(handleFeatureNotSupported)

	address := fmt.Sprintf("%s:%d", config.BindAddress, config.Port)

	server := &http.Server{
		Addr:    address,
		Handler: router,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, os.Signal(syscall.SIGTERM), syscall.SIGINT)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logrus.WithFields(logrus.Fields{
				"error": err,
			}).Error("ListenAndServe error")
		}
	}()

	logrus.WithFields(logrus.Fields{
		"address": address,
		"started": time.Now(),
	}).Info("InfluxDB proxy is running")

	received := <-quit

	logrus.WithFields(logrus.Fields{
		"address": address,
		"stopped": time.Now(),
		"signal":  received,
	}).Info("Shutting down InfluxDB proxy")

	for _, instance := range instances {
		close(instance.Channel)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err,
		}).Error("Error shutting down server")
	}
	logrus.Info("InfluxDB proxy stopped")
}

func createInstances(addresses []Address) []InfluxDBInstance {
	instances := make([]InfluxDBInstance, len(addresses))
	for i, address := range addresses {
		ch := make(chan Payload, config.ChannelSize)
		instances[i] = InfluxDBInstance{URL: address.Url, Token: address.Token, Channel: ch}
		go handleRequests(ch, address)
	}
	return instances
}

func handleRequests(requests <-chan Payload, influxDBURL Address) {
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		},
	}
	for req := range requests {
		url := influxDBURL.Url + req.URLPath
		url += "?" + req.QueryParams.Encode()
		newReq, err := http.NewRequest(req.Method, url, strings.NewReader(string(req.Body)))
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"error": err,
				"url":   influxDBURL.Url,
			}).Error("Error creating request")
			continue
		}
		newReq.Header = req.Header.Clone()
		if influxDBURL.Token != "" {
			newReq.Header["Authorization"] = []string{"Token " + influxDBURL.Token}
		}

		retryCount := 0
		maxRetries := config.RetryCount
		for {
			resp, err := client.Do(newReq)
			if err != nil {
				if retryCount < maxRetries {
					retryCount++
					time.Sleep(time.Duration(config.RetryDelay) * time.Second)
					continue
				}
				logrus.WithFields(logrus.Fields{
					"error": err,
					"url":   influxDBURL.Url,
				}).Error("Max retries exceeded")
				break
			}
			// Print response body
			if resp.StatusCode > 299 {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					logrus.WithFields(logrus.Fields{
						"error": err,
						"url":   influxDBURL.Url,
					}).Error("Error reading response body")
				}
				logrus.WithFields(logrus.Fields{
					"status": resp.StatusCode,
					"body":   string(body),
				}).Debug("InfluxDB response")
			}
			defer resp.Body.Close()
			logrus.WithFields(logrus.Fields{
				"status": resp.StatusCode,
				"url":    influxDBURL.Url,
			}).Info("InfluxDB response")
			break
		}
	}
}
