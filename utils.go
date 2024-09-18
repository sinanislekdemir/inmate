package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"slices"
	"time"
)

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
