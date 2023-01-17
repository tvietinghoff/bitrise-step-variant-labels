package main

import (
	"bytes"
	"fmt"
	"github.com/bitrise-io/go-utils/log"
	"net/http"
	"strings"
)

func ApiRequest(url string, conf conf, method string, requestBody string) (error, string) {

	request, err := http.NewRequest(method, url, strings.NewReader(requestBody))
	if err != nil {
		fail("failed to create request: %v\n", err)
	}
	request.Header.Add("Content-Type", "application/json")
	request.Header.Add("Authorization", strings.Replace("Bearer $AuthToken", "$AuthToken", conf.AuthToken, 1))
	request.Header.Add("User-Agent", "tvietinghoff/bitrise-step-variant-labels")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		fail("failed to send api request: %v\n", err)
	}
	if response.StatusCode >= 300 {
		buf := new(bytes.Buffer)
		_, err = buf.ReadFrom(response.Body)
		if err != nil {
			log.Warnf("api request returned %v\n%v\n"+response.Status, err)
		} else {
			err = fmt.Errorf("api request returned %v\n" + response.Status)
		}
		return err, ""
	}
	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(response.Body)
	if err != nil {
		log.Warnf("failed to read response %v\n", err)
	}
	jsonResponse := buf.String()
	return err, jsonResponse
}
