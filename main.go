package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v2"
)

type InfluxDBConfig struct {
	URLs         []string `yaml:"urls"`
	Port         int      `yaml:"port"`
	BindAddress  string   `yaml:"bind_address"`
	RetryDelay   int      `yaml:"retry_delay"`
	RetryCount   int      `yaml:"retry_count"`
	QueryTimeout int      `yaml:"query_timeout"`
	ChannelSize  int      `yaml:"channel_size"`
}

type Payload struct {
	Body        []byte
	URLPath     string
	Method      string
	QueryParams url.Values
	Header      http.Header
}

type InfluxDBInstance struct {
	URL     string
	Channel chan Payload
}

func backFillHeaders(c *gin.Context, resp *http.Response) {
	for key, values := range resp.Header {
		for _, value := range values {
			c.Header(key, value)
		}
	}
}

var config InfluxDBConfig

func main() {
	loadConfig("config.yaml")
	instances := createInstances(config.URLs)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(MaskedLogger())

	router.POST("/write", handleWrite(instances))
	router.POST("/api/v2/write", handleWrite(instances))
	router.GET("/ping", handlePing(instances))
	router.POST("/query", handleQuery(instances))
	router.GET("/query", handleQuery(instances))
	router.GET("/", handleHealthCheck)

	address := fmt.Sprintf("%s:%d", config.BindAddress, config.Port)

	server := &http.Server{
		Addr:    address,
		Handler: router,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, os.Signal(syscall.SIGTERM), syscall.SIGINT)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("ListenAndServe error: %s\n", err)
		}
	}()
	log.Printf("InfluxDB proxy is running on %s\n", address)
	<-quit
	log.Println("Shutting down InfluxDB proxy")

	for _, instance := range instances {
		close(instance.Channel)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Error shutting down server: %v", err)
	}
	log.Println("InfluxDB proxy stopped")
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

func loadConfig(filename string) {
	data, err := os.ReadFile(filename)
	if err != nil {
		log.Fatalf("Error reading config file: %v", err)
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		log.Fatalf("Error parsing config file: %v", err)
	}
}

func createInstances(urls []string) []InfluxDBInstance {
	instances := make([]InfluxDBInstance, len(urls))
	for i, url := range urls {
		ch := make(chan Payload, config.ChannelSize)
		instances[i] = InfluxDBInstance{URL: url, Channel: ch}
		go handleRequests(ch, url)
	}
	return instances
}

func handleWrite(instances []InfluxDBInstance) gin.HandlerFunc {
	return func(c *gin.Context) {
		handleRequestPayload(c, instances)
		c.JSON(http.StatusNoContent, nil)
	}
}

func handleHealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, map[string]interface{}{"status": "ok", "message": "InfluxDB proxy is running", "active_instances": len(config.URLs)})
}

func handlePing(instances []InfluxDBInstance) gin.HandlerFunc {
	return func(c *gin.Context) {
		resp, err := sendRequestRandomInstance(instances, c.Request.URL.Path, c.Request.URL.Query(), c.Request)
		if err != nil {
			handleError(c, err)
			return
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			handleError(c, err)
			return
		}

		backFillHeaders(c, resp)
		c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), body)
	}
}

func handleQuery(instances []InfluxDBInstance) gin.HandlerFunc {
	return func(c *gin.Context) {
		queryParams := c.Request.URL.Query()
		if isMutation(queryParams.Get("q")) {
			handleMutation(c, instances)
			return
		}

		resp, err := sendRequestRandomInstance(instances, c.Request.URL.Path, queryParams, c.Request)
		if err != nil {
			log.Printf("Error sending request: %v\n", err)
			handleError(c, err)
			return
		}
		defer resp.Body.Close()

		backFillHeaders(c, resp)
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			handleError(c, err)
			return
		}
		c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), body)
	}
}

func isMutation(query string) bool {
	mutations := []string{"CREATE", "DROP", "ALTER", "GRANT", "REVOKE", "INSERT", "UPDATE", "DELETE"}
	for _, mutation := range mutations {
		if strings.HasPrefix(strings.ToUpper(query), mutation) {
			return true
		}
	}
	return false
}

func handleMutation(c *gin.Context, instances []InfluxDBInstance) {
	queryParams := c.Request.URL.Query()
	isFirst := true
	allGood := true
	var body []byte

	for _, instance := range instances {
		url := instance.URL + c.Request.URL.Path
		resp, err := sendRequestWithRetry(url, queryParams, c.Request, true)
		if err != nil {
			handleError(c, err)
			return
		}
		if resp.StatusCode > 299 {
			allGood = false
		}
		if isFirst {
			defer resp.Body.Close()
			body, err = io.ReadAll(resp.Body)
			if err != nil {
				handleError(c, err)
				return
			}
			backFillHeaders(c, resp)
			isFirst = false
		} else {
			resp.Body.Close()
		}
	}

	if allGood {
		c.Data(http.StatusOK, "application/json", body)
	} else {
		c.JSON(http.StatusInternalServerError, map[string]string{"error": "Some instances failed to execute the query"})
	}
}

func handleRequestPayload(c *gin.Context, instances []InfluxDBInstance) {
	body, err := c.GetRawData()
	if err != nil {
		handleError(c, err)
		return
	}
	payload := Payload{
		Body:        body,
		URLPath:     c.Request.URL.Path,
		Method:      c.Request.Method,
		Header:      c.Request.Header,
		QueryParams: c.Request.URL.Query(),
	}
	for _, instance := range instances {
		instance.Channel <- payload
		log.Println("Request sent to", instance.URL)
	}
}

func handleError(c *gin.Context, err error) {
	log.Println("Error:", err)
	c.JSON(http.StatusInternalServerError, nil)
}

func sendRequestRandomInstance(instances []InfluxDBInstance, url string, queryValues url.Values, request *http.Request) (*http.Response, error) {
	if len(instances) == 0 {
		return nil, fmt.Errorf("all instances are down")
	}

	choice := rand.Intn(len(instances))
	instance := instances[choice]

	// remove selected instance to prevent reselected againg.
	instances = slices.Concat(instances[:choice], instances[choice+1:])

	log.Printf("Sending request to %s\n", instance.URL)
	resp, err := sendRequestWithRetry(instance.URL+url, queryValues, request, false)
	if err != nil {
		return sendRequestRandomInstance(instances, url, queryValues, request)
	}

	return resp, err
}

func sendRequestWithRetry(url string, queryValues url.Values, request *http.Request, enforce bool) (*http.Response, error) {
	log.Printf("Sending request to %s, query string %s\n", url, queryValues.Get("q"))
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		},
		Timeout: time.Duration(config.QueryTimeout) * time.Second,
	}
	fullUrl := url + "?" + queryValues.Encode()
	newReq, err := http.NewRequest(request.Method, fullUrl, request.Body)
	if err != nil {
		return nil, err
	}
	newReq.Header = request.Header.Clone()

	retryCount := 0
	maxRetries := config.RetryCount
	for {
		resp, err := client.Do(newReq.WithContext(request.Context()))
		if err != nil {
			log.Printf("Error sending request: %v\n", err)
			if !enforce {
				return nil, err
			}
			if retryCount < maxRetries {
				retryCount++
				log.Printf("Retrying request to %s\n", url)
				time.Sleep(time.Duration(config.RetryDelay) * time.Second)
				continue
			}
			return nil, err
		}
		log.Printf("InfluxDB [%s] response: %d\n", url, resp.StatusCode)
		return resp, nil
	}
}

func handleRequests(requests <-chan Payload, influxDBURL string) {
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		},
	}
	for req := range requests {
		url := influxDBURL + req.URLPath
		url += "?" + req.QueryParams.Encode()
		newReq, err := http.NewRequest(req.Method, url, strings.NewReader(string(req.Body)))
		if err != nil {
			log.Println("Error creating request:", err)
			continue
		}
		newReq.Header = req.Header.Clone()

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
				log.Println("Max retries exceeded")
				break
			}
			// Print response body
			if resp.StatusCode > 299 {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					log.Println("Error reading response body:", err)
				}
				log.Println("Response body:", string(body))
			}
			defer resp.Body.Close()
			log.Printf("InfluxDB [%s] response: %d\n", influxDBURL, resp.StatusCode)
			break
		}
	}
}
