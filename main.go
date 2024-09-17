package main

import (
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v2"
)

type InfluxDBConfig struct {
	URLs        []string `yaml:"urls"`
	Port        int      `yaml:"port"`
	BindAddress string   `yaml:"bind_address"`
	RetryDelay  int      `yaml:"retry_delay"`
	RetryCount  int      `yaml:"retry_count"`
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
	router := gin.Default()

	router.POST("/write", handleWrite(instances))
	router.POST("/api/v2/write", handleWrite(instances))
	router.GET("/ping", handlePing(instances))
	router.POST("/query", handleQuery(instances))

	address := fmt.Sprintf("%s:%d", config.BindAddress, config.Port)

	if err := router.Run(address); err != nil {
		log.Fatal(err)
	}
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
		ch := make(chan Payload, 100)
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

func handlePing(instances []InfluxDBInstance) gin.HandlerFunc {
	return func(c *gin.Context) {
		randomInstance := instances[rand.Intn(len(instances))]
		url := randomInstance.URL + c.Request.URL.Path
		resp, err := sendRequestWithRetry(url, c.Request.URL.Query(), c.Request)
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

		randomInstance := instances[rand.Intn(len(instances))]
		url := randomInstance.URL + c.Request.URL.Path
		resp, err := sendRequestWithRetry(url, queryParams, c.Request)
		if err != nil {
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
		resp, err := sendRequestWithRetry(url, queryParams, c.Request)
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
		fmt.Println("Request buffered for", instance.URL)
	}
}

func handleError(c *gin.Context, err error) {
	log.Println("Error:", err)
	c.JSON(http.StatusInternalServerError, nil)
}

func sendRequestWithRetry(url string, queryValues url.Values, request *http.Request) (*http.Response, error) {
	log.Printf("Sending request to %s with query values: %v\n", url, queryValues)
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		},
	}
	url += "?" + queryValues.Encode()
	newReq, err := http.NewRequest(request.Method, url, request.Body)
	if err != nil {
		return nil, err
	}
	newReq.Header = request.Header.Clone()

	// TODO: Implement exponential backoff and increase the max retries.
	// We can even wait for the InfluxDB to be up and running before sending the requests.

	retryCount := 0
	maxRetries := config.RetryCount
	for {
		resp, err := client.Do(newReq.WithContext(request.Context()))
		if err != nil {
			if retryCount < maxRetries {
				retryCount++
				time.Sleep(time.Duration(config.RetryDelay) * time.Second)
				continue
			}
			return nil, err
		}
		log.Println("InfluxDB response:", resp.StatusCode)
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

		// TODO: Implement exponential backoff and increase the max retries.
		// We can even wait for the InfluxDB to be up and running before sending the requests.

		retryCount := 0
		maxRetries := 60
		for {
			resp, err := client.Do(newReq)
			if err != nil {
				if retryCount < maxRetries {
					retryCount++
					time.Sleep(1 * time.Second)
					continue
				}
				log.Println("Max retries exceeded")
				break
			}
			defer resp.Body.Close()
			log.Println("InfluxDB response:", resp.StatusCode)
			break
		}
	}
}
