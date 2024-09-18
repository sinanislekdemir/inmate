package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/sirupsen/logrus"
)

func sendRequestRandomInstance(instances []InfluxDBInstance, url string, queryValues url.Values, request *http.Request) (*http.Response, error) {
	if len(instances) == 0 {
		return nil, fmt.Errorf("all instances are down")
	}

	choice := rand.Intn(len(instances))
	instance := instances[choice]

	// remove selected instance to prevent reselected againg.
	instances = slices.Concat(instances[:choice], instances[choice+1:])

	logrus.WithFields(logrus.Fields{
		"instance": instance.URL,
	}).Info("Sending request")
	resp, err := sendRequestWithRetry(instance.URL+url, instance.Token, queryValues, request, false)
	if err != nil {
		return sendRequestRandomInstance(instances, url, queryValues, request)
	}

	return resp, err
}

func sendRequestWithRetry(url string, token string, queryValues url.Values, request *http.Request, enforce bool) (*http.Response, error) {
	logrus.WithFields(logrus.Fields{
		"url": url,
		"q":   queryValues.Get("q"),
	}).Info("Sending request")

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

	if token != "" {
		newReq.Header["Authorization"] = []string{"Token " + token}
	}

	retryCount := 0
	maxRetries := config.RetryCount
	for {
		resp, err := client.Do(newReq.WithContext(request.Context()))
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"error": err,
				"url":   url,
			}).Error("Error sending request")

			if !enforce {
				return nil, err
			}
			if retryCount < maxRetries {
				retryCount++
				logrus.WithFields(logrus.Fields{
					"retryCount": retryCount,
					"url":        url,
				}).Info("Retrying request")
				time.Sleep(time.Duration(config.RetryDelay) * time.Second)
				continue
			}
			return nil, err
		}
		logrus.WithFields(logrus.Fields{
			"status": resp.StatusCode,
			"url":    url,
		}).Info("InfluxDB response")
		return resp, nil
	}
}
