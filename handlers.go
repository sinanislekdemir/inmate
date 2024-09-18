package main

import (
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

func backFillHeaders(c *gin.Context, resp *http.Response) {
	for key, values := range resp.Header {
		for _, value := range values {
			c.Header(key, value)
		}
	}
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
			logrus.WithFields(logrus.Fields{
				"error": err,
			}).Error("Error sending request")
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
		logrus.WithFields(logrus.Fields{
			"url": instance.URL,
		}).Info("Request sent")
	}
}

func handleError(c *gin.Context, err error) {
	logrus.WithFields(logrus.Fields{
		"error": err,
	}).Error("Error processing request")

	c.JSON(http.StatusInternalServerError, nil)
}
