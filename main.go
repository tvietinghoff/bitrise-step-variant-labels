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
	RepoOwner       string `env:"repo_owner,required"`
	RepoName        string `env:"repo_name,required"`
	AuthToken       string `env:"auth_token,required"`
	PullRequest     int    `env:"pull_request"`
	CommitHash      string `env:"commit_hash"`
	FlavorLabels    string `env:"flavor_labels,required"`
	VariantPatterns string `env:"variant_patterns,required"`
}

type PRGraphQLResponse struct {
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

type MergeGraphQLResponse struct {
	Data struct {
		Repository struct {
			Object struct {
				PullRequests struct {
					Edges []struct {
						Node struct {
							Labels struct {
								Edges []struct {
									Node struct {
										Name string `json:"name"`
									} `json:"node"`
								} `json:"edges"`
							} `json:"labels"`
						} `json:"node"`
					} `json:"edges"`
				} `json:"associatedPullRequests"`
			} `json:"object"`
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

	variantPatternRegex := regexp.MustCompile(`#\d`)
	variantPatterns := make(map[string]string)

	for _, patternSpec := range strings.Split(conf.VariantPatterns, "|") {
		parts := strings.Split(patternSpec, "=")
		if len(parts) != 2 {
			fail("invalid variant pattern specification: %v\nExpected '{variable}={pattern}[;{separator}]", patternSpec)
		}

		key := strings.TrimSpace(parts[0])
		if len(key) == 0 {
			fail("variant pattern specification does not include a key, check input: %v", patternSpec)
		}
		pattern := strings.TrimSpace(parts[1])

		if !variantPatternRegex.MatchString(pattern) {
			fail("variant pattern does not include a placeholder #<n>, check input: %v", patternSpec)
		}

		variantPatterns[key] = pattern
	}

	if conf.PullRequest != 0 {
		fetchFlavorDimensionsForPR(conf, flavors, flavorDimensions)
	} else if conf.CommitHash != "" {
		fetchFlavorDimensionsForCommit(conf, flavors, flavorDimensions)
	} else {
		log.Warnf("Neither commit_hash nor pull_request given. Building defaults only.")
		for index, dimension := range flavorDimensions {
			if dimension.DefaultFlavor == "" {
				fail("Missing default for flavor dimension %d, aborting...", index)
			}
		}
	}

	for key, pattern := range variantPatterns {
		generateEnvironmentVariable(key, pattern, flavorDimensions)
	}

	os.Exit(0)
}

func fetchFlavorDimensionsForPR(conf Conf, flavors map[string]int, flavorDimensions map[int]FlavorDimension) {
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
	err, jsonResponse := graphQLRequest(requestBody, replacements, conf)
	var graphQLResponse PRGraphQLResponse
	err = json.NewDecoder(strings.NewReader(jsonResponse)).Decode(&graphQLResponse)
	if err != nil {
		fail("failed to decode graphql response: %v\n", err)
	}
	for _, label := range graphQLResponse.Data.Repository.PullRequest.Labels.Edges {
		labelName := label.Node.Name
		dimension := flavors[labelName]
		if dimension != 0 {
			flavorDimensions[dimension].SelectedFlavors[labelName] = true
			fmt.Printf("Found label for flavor %s\n", labelName)
		}
	}
}

func fetchFlavorDimensionsForCommit(conf Conf, flavors map[string]int, flavorDimensions map[int]FlavorDimension) {
	requestBody := `
	{ "query": 
		"{
			repository(owner: \"$RepoOwner\", name: \"$RepoName\") {
			    object(oid:\"$Commit\"){
					... on Commit{
						associatedPullRequests(last: 1){
							edges{
								node{
									labels(first: 50) {
										edges {
											node {
												name
											}
										}
									}
								}
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
		"$Commit", conf.CommitHash,
		"\n", " ",
		"\t", ""}
	err, jsonResponse := graphQLRequest(requestBody, replacements, conf)
	var graphQLResponse MergeGraphQLResponse
	err = json.NewDecoder(strings.NewReader(jsonResponse)).Decode(&graphQLResponse)
	if err != nil {
		fail("failed to decode graphql response: %v\n", err)
	}
	if len(graphQLResponse.Data.Repository.Object.PullRequests.Edges) == 0 {
		log.Warnf("No associated pull request found, applying defaults...", err)
		return
	}
	for _, label := range graphQLResponse.Data.Repository.Object.PullRequests.Edges[0].Node.Labels.Edges {
		labelName := label.Node.Name
		dimension := flavors[labelName]
		if dimension != 0 {
			flavorDimensions[dimension].SelectedFlavors[labelName] = true
			fmt.Printf("Found label for flavor %s\n", labelName)
		}
	}
}

func graphQLRequest(requestBody string, replacements []string, conf Conf) (error, string) {
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
	return err, jsonResponse
}

func generateEnvironmentVariable(key string, pattern string, flavorDimensions map[int]FlavorDimension) {
	patterns := make(map[string]bool)
	separator := " "
	separatorPos := strings.Index(pattern, `;`)
	if separatorPos > 0 {
		separator = pattern[separatorPos+1:]
		if len(separator) == 0 {
			separator = " "
		}
		pattern = pattern[:separatorPos]
	}
	pattern = strings.TrimSpace(pattern)

	patterns[pattern] = true
	for index, flavorDimension := range flavorDimensions {
		outPatterns := make(map[string]bool)
		placeholder := fmt.Sprintf("#%d", flavorDimension.Index)
		selectedFlavors := flavorDimension.SelectedFlavors
		if len(selectedFlavors) == 0 {
			selectedFlavors = make(map[string]bool)
			selectedFlavors[flavorDimension.DefaultFlavor] = true
			fmt.Printf("No label for flavor dimension %d found, defaulting to %s\n", index, flavorDimension.DefaultFlavor)
		}
		for flavorLabel := range selectedFlavors {
			flavor := flavorDimension.Flavors[flavorLabel]
			for pattern := range patterns {
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
	// finally, patterns contains all combinations of pattern with resolved placeholders
	variants := make([]string, len(patterns))
	i := 0
	for variant := range patterns {
		variants[i] = variant
		i++
	}
	variantsString := strings.Join(variants, separator)
	fmt.Printf("%s = %s\n", key, variantsString)
	err := tools.ExportEnvironmentWithEnvman(key, variantsString)
	if err != nil {
		fail("Failed to export environment variable: %v", err)
	}
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
