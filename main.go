package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/bitrise-io/go-steputils/tools"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-tools/go-steputils/stepconf"
	"net/http"
	"os"
	"regexp"
	"strings"
)

type Conf struct {
	RepoOwner      string `env:"repo_owner,required"`
	RepoName       string `env:"repo_name,required"`
	AuthToken      string `env:"auth_token,required"`
	PullRequest    int    `env:"pull_request,required"`
	FlavorLabels   string `env:"flavor_labels,required"`
	VariantPattern string `env:"variant_pattern,required"`
}

type GraphQLResponse struct {
	Data struct {
		Repository struct {
			PullRequest struct {
				Labels struct {
					Edges []struct {
						Node struct {
							Name string `json:"name"`
						} `json:"node"`
					} `json:"edges"`
				} `json:"labels"`
			} `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
}

func fail(message string, args ...interface{}) {
	log.Errorf(message, args...)
	os.Exit(1)
}

func main() {

	var conf Conf

	if err := stepconf.Parse(&conf); err != nil {
		fail("step config failed: %v\n", err)
	}
	printconf := conf
	printconf.AuthToken = "***"

	stepconf.Print(printconf)

	flavorDimensions, flavors := getFlavorDimensions(conf)
	if len(flavorDimensions) == 0 {
		fail("failed to parse flavor labels, check input: %v", conf.FlavorLabels)
	}
	variantPatternRegex := regexp.MustCompile(`\$\d`)
	if !variantPatternRegex.MatchString(conf.VariantPattern) {
		fail("variant pattern does not include a placeholder $<n>, check input: %v", conf.VariantPattern)
	}

	requestBody := `
{ "query": 
	"{
		repository(owner: \"$RepoOwner\", name: \"$RepoName\") {
		    pullRequest(number: $PullRequest) {
      			labels(first: 50) {
        			edges {
          				node {
            				name
          				}
        			}
      			}
    		}
  		}
	}"
}`
	replacements := []string{
		"$RepoOwner", conf.RepoOwner,
		"$RepoName", conf.RepoName,
		"$PullRequest", fmt.Sprintf("%d", conf.PullRequest),
		"\n", " ",
		"\t", ""}
	requestBody = strings.NewReplacer(replacements...).Replace(requestBody)
	request, err := http.NewRequest("POST", "https://api.github.com/graphql", strings.NewReader(requestBody))
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
	var graphQLResponse GraphQLResponse

	err = json.NewDecoder(strings.NewReader(jsonResponse)).Decode(&graphQLResponse)
	if err != nil {
		fail("failed to decode graphql response: %v\n", err)
	}

	for _, label := range graphQLResponse.Data.Repository.PullRequest.Labels.Edges {
		dimension := flavors[label.Node.Name]
		if dimension != 0 {
			flavorDimensions[dimension].SelectedFlavors[label.Node.Name] = true
		}
	}

	patterns := make(map[string]bool)
	patterns[strings.Trim(conf.VariantPattern, " ")] = true

	for _, flavorDimension := range flavorDimensions {
		outPatterns := make(map[string]bool)
		placeholder := fmt.Sprintf("$%d", flavorDimension.Index)
		selectedFlavors := flavorDimension.SelectedFlavors
		if len(selectedFlavors) == 0 {
			selectedFlavors = make(map[string]bool)
			selectedFlavors[flavorDimension.DefaultFlavor] = true
		}
		for flavorLabel, _ := range selectedFlavors {
			flavor := flavorDimension.Flavors[flavorLabel]
			for pattern, _ := range patterns {
				var outPattern = pattern
				if strings.HasPrefix(pattern, placeholder) {
					outPattern = flavor + strings.TrimPrefix(pattern, placeholder)
				}
				outPattern = strings.ReplaceAll(outPattern, placeholder, strings.ToUpper(flavor[:1])+flavor[1:])
				outPatterns[outPattern] = true
			}
		}
		patterns = outPatterns
	}
	// finally, patterns contains all combinations of conf.VariantPattern with resolved placeholders
	variants := make([]string, len(patterns))
	i := 0
	for variant, _ := range patterns {
		variants[i] = variant
		i++
	}
	variantsString := strings.Join(variants, " ")
	fmt.Printf("variants = %s\n", variants)
	err = tools.ExportEnvironmentWithEnvman(`VARIANTS`, variantsString)
	if err != nil {
		fail("Failed to export environment variable: %v", err)
	}
	os.Exit(0)
}

type FlavorDimension struct {
	Index           int
	Flavors         map[string]string
	DefaultFlavor   string
	SelectedFlavors map[string]bool
}

func getFlavorDimensions(conf Conf) (map[int]FlavorDimension, map[string]int) {
	flavorDimensions := make(map[int]FlavorDimension)
	flavors := make(map[string]int)
	for i, group := range strings.Split(conf.FlavorLabels, "|") {
		index := i + 1
		for _, label := range strings.Split(strings.Trim(group, " "), ",") {
			label = strings.Trim(label, " ")
			isDefault := strings.HasPrefix(label, "!")
			if isDefault {
				label = strings.TrimPrefix(label, "!")
			}

			flavorNamePos := strings.Index(label, "=")
			flavorName := label
			if flavorNamePos >= 0 {
				flavorName = label[flavorNamePos+1:]
				label = label[:flavorNamePos]
			}

			flavorDimension := flavorDimensions[index]
			if flavorDimension.Index == 0 {
				flavorDimension.Index = index
				flavorDimension.Flavors = make(map[string]string)
				flavorDimension.SelectedFlavors = make(map[string]bool)
			}
			flavors[label] = index
			flavorDimension.Flavors[label] = flavorName
			if isDefault {
				flavorDimension.DefaultFlavor = label
			}
			flavorDimensions[index] = flavorDimension
		}
	}
	return flavorDimensions, flavors
}
