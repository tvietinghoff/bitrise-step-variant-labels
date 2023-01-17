package main

import (
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
	return ApiRequest(url, conf, "POST", requestBody)
}
