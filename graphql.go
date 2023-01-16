package main

import (
	"bytes"
	"net/http"
	"strings"
)

func GraphQlRequest(requestBody string, replacements []string, conf conf) (error, string) {
	requestBody = strings.NewReplacer(replacements...).Replace(requestBody)
	url := ""
	if conf.Provider == "github" {
		url = "https://api.github.com/graphql"
	} else if conf.Provider == "gitlab" {
		url = "https://gitlab.com/api/graphql"
	} else {
		fail("Invalid provider %v", conf.Provider)
	}
	request, err := http.NewRequest("POST", url, strings.NewReader(requestBody))
	if err != nil {
		fail("failed to create request: %v\n", err)
	}
	request.Header.Add("Content-Type", "application/json")
	request.Header.Add("Authorization", strings.Replace("Bearer $AuthToken", "$AuthToken", conf.AuthToken, 1))
	request.Header.Add("User-Agent", "tvietinghoff/bitrise-step-variant-labels")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		fail("failed to send graphql request: %v\n", err)
	}
	if response.StatusCode != 200 {
		buf := new(bytes.Buffer)
		_, err = buf.ReadFrom(response.Body)
		if err != nil {
			fail("graphql request returned %v\n%v\n"+response.Status, err)
		}
		fail("graphql request returned %v\n%v\n"+response.Status, buf.String())
	}
	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(response.Body)
	if err != nil {
		fail("failed to read response %v\n", err)
	}
	jsonResponse := buf.String()
	return err, jsonResponse
}