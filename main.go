package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/bitrise-io/go-steputils/tools"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-tools/go-steputils/stepconf"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Conf struct {
	Provider          string `env:"provider"`
	ProjectPath       string `env:"project_path"`
	RepoOwner         string `env:"repo_owner"`
	RepoName          string `env:"repo_name"`
	AuthToken         string `env:"auth_token,required"`
	PullRequest       int    `env:"pull_request"`
	CommitHash        string `env:"commit_hash"`
	VariantLabels     string `env:"variant_labels,required"`
	VariantPatterns   string `env:"variant_patterns,required"`
	ExportDescription string `env:"export_description"`
	Labels2Env        string `env:"labels2env"`
}

type PRGraphQLResponseGithub struct {
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
type PRGraphQLResponseGitlab struct {
	Data struct {
		Project struct {
			MergeRequest MergeRequestGitlab `json:"mergeRequest"`
			/*			MergeRequest struct {
							Description     string `json:"description"`
							DescriptionHtml string `json:"descriptionHtml"`
							MergeCommitSha  string `json:"mergeCommitSha"`
							Labels          struct {
								Edges []struct {
									Node struct {
										Title string `json:"title"`
									} `json:"node"`
								} `json:"edges"`
							} `json:"labels"`
						} `json:"mergeRequest"`
			*/
		} `json:"project"`
	} `json:"data"`
}

type MergeGraphQLResponseGithub struct {
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

type MergeRequestLabelGitlab struct {
	Node struct {
		Title string `json:"title"`
	} `json:"node"`
}
type MergeRequestGitlab struct {
	Description     string `json:"description"`
	DescriptionHtml string `json:"descriptionHtml"`
	Title           string `json:"title"`
	TitleHtml       string `json:"titleHtml"`
	MergeCommitSha  string `json:"mergeCommitSha"`
	Labels          struct {
		Edges []MergeRequestLabelGitlab `json:"edges"`
	} `json:"labels"`
}
type MergeRequestGitlabEdge struct {
	Node MergeRequestGitlab `json:"node"`
}

type MergeGraphQLResponseGitlab struct {
	Data struct {
		Project struct {
			MergeRequests struct {
				Edges []MergeRequestGitlabEdge `json:"edges"`
			} `json:"mergeRequests"`
		} `json:"project"`
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

	if len(conf.Provider) == 0 {
		conf.Provider = "github"
	}

	if conf.Provider == "github" {
		if len(conf.RepoName) == 0 {
			fail("Missing repo name argument")
		}
		if len(conf.RepoOwner) == 0 {
			fail("Missing repo owner argument")
		}
	} else if conf.Provider == "gitlab" {
		if len(conf.ProjectPath) == 0 {
			fail("Missing project path argument")
		}
	} else {
		fail("Invalid provider: %v. Allowed are: github, gitlab", conf.Provider)
	}

	flavorDimensions, flavors := getFlavorDimensions(conf)
	if len(flavorDimensions) == 0 {
		fail("failed to parse flavor labels, check input: %v", conf.VariantLabels)
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

	var labels = make(map[string]bool)
	if conf.PullRequest != 0 {
		labels = fetchFlavorDimensionsForPR(conf, flavors, flavorDimensions)
	} else if conf.CommitHash != "" {
		labels = fetchFlavorDimensionsForCommit(conf, flavors, flavorDimensions)
	} else {
		log.Warnf("Neither commit_hash nor pull_request given. Building defaults only.")
		for index, dimension := range flavorDimensions {
			if dimension.DefaultFlavor == "" {
				fail("Missing default for flavor dimension %d, aborting...", index)
			}
		}
		labels = nil
	}

	label2Env(conf, labels)

	for key, pattern := range variantPatterns {
		generateEnvironmentVariable(key, pattern, flavorDimensions)
	}

	os.Exit(0)
}

func fetchFlavorDimensionsForPR(conf Conf, flavors map[string]int, flavorDimensions map[int]FlavorDimension) map[string]bool {
	if conf.Provider == "github" {
		return fetchFlavorDimensionsForPRGithub(conf, flavors, flavorDimensions)
	} else if conf.Provider == "gitlab" {
		return processPRGitlab(conf, flavors, flavorDimensions)
	} else {
		// should not be reached, provider is checked up front
		fail("Invalid provider %v", conf.Provider)
	}
	return nil
}
func fetchFlavorDimensionsForCommit(conf Conf, flavors map[string]int, flavorDimensions map[int]FlavorDimension) map[string]bool {
	if conf.Provider == "github" {
		return fetchFlavorDimensionsForCommitGithub(conf, flavors, flavorDimensions)
	} else if conf.Provider == "gitlab" {
		return fetchFlavorDimensionsForCommitGitlab(conf, flavors, flavorDimensions)
	} else {
		// should not be reached, provider is checked up front
		fail("Invalid provider %v", conf.Provider)
		return nil
	}
}
func fetchFlavorDimensionsForPRGithub(conf Conf, flavors map[string]int, flavorDimensions map[int]FlavorDimension) map[string]bool {
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
	var graphQLResponse PRGraphQLResponseGithub
	err = json.NewDecoder(strings.NewReader(jsonResponse)).Decode(&graphQLResponse)
	if err != nil {
		fail("failed to decode graphql response: %v\n", err)
	}
	var labels = make(map[string]bool)
	for _, label := range graphQLResponse.Data.Repository.PullRequest.Labels.Edges {
		labelName := label.Node.Name
		dimension := flavors[labelName]
		if dimension != 0 {
			flavorDimensions[dimension].SelectedFlavors[labelName] = true
			fmt.Printf("Found label for flavor %s\n", labelName)
		}
		labels[labelName] = true
	}
	return labels
}

func processPRGitlab(conf Conf, flavors map[string]int, flavorDimensions map[int]FlavorDimension) map[string]bool {
	mergeRequest := fetchMergeRequestForPRGitlab(conf)

	if mergeRequest == nil {
		log.Warnf("Merge request not found, applying defaults...")
		return nil
	}

	maybeExportDescription(conf, *mergeRequest)

	return processFlavorsGitlab(*mergeRequest, flavors, flavorDimensions)
}

func processFlavorsGitlab(mergeRequest MergeRequestGitlab, flavors map[string]int, flavorDimensions map[int]FlavorDimension) map[string]bool {
	mrLabels := mergeRequest.Labels.Edges
	if len(mrLabels) == 0 {
		log.Warnf("No labels found, applying defaults...")
		return nil
	}
	var labels = make(map[string]bool)
	for _, label := range mrLabels {
		labelName := label.Node.Title
		dimension := flavors[labelName]
		if dimension != 0 {
			flavorDimensions[dimension].SelectedFlavors[labelName] = true
			variant := flavorDimensions[dimension].Flavors[labelName]
			fmt.Printf("Found label for variant %s\n", variant)
		}
		labels[labelName] = true
	}
	return labels
}

func fetchMergeRequestForPRGitlab(conf Conf) *MergeRequestGitlab {
	requestBody := `
	{ "query":
		"query {
			project(fullPath: \"$ProjectPath\") {
				mergeRequest(iid: \"$PullRequest\") {
					title,
					titleHtml,
					description,
					descriptionHtml,
	  		  		mergeCommitSha,
					labels {
						edges {
		  					node {
								title
		  					}
						}
					}
				}
			}
	  	}"
	}`
	replacements := []string{
		"$ProjectPath", conf.ProjectPath,
		"$PullRequest", fmt.Sprintf("%d", conf.PullRequest),
		"\n", " ",
		"\t", ""}
	err, jsonResponse := graphQLRequest(requestBody, replacements, conf)
	var graphQLResponse PRGraphQLResponseGitlab
	err = json.NewDecoder(strings.NewReader(jsonResponse)).Decode(&graphQLResponse)
	if err != nil {
		fail("failed to decode graphql response: %v\n", err)
	}
	return &graphQLResponse.Data.Project.MergeRequest
}

func maybeExportDescription(conf Conf, mergeRequest MergeRequestGitlab) {
	if len(conf.ExportDescription) == 0 {
		return
	}
	description := mergeRequest.Title + "\n\n" + mergeRequest.Description
	html := mergeRequest.TitleHtml + "<br><br>" + mergeRequest.DescriptionHtml

	ext := filepath.Ext(conf.ExportDescription)
	if len(ext) == 0 || strings.ToLower(ext) == ".txt" {
		if len(description) == 0 {
			log.Warnf("Text description not available, but export was requested")
		} else {
			path := strings.TrimSuffix(conf.ExportDescription, ".txt") + ".txt"
			ioutil.WriteFile(path, []byte(description), 0644)
		}
	}
	if len(ext) == 0 || strings.ToLower(ext) == ".html" {
		if len(html) == 0 {
			log.Warnf("HTML description not available, but export was requested")
		} else {
			path := strings.TrimSuffix(conf.ExportDescription, ".html") + ".html"
			ioutil.WriteFile(path, []byte(html), 0644)
		}
	}
}

func fetchFlavorDimensionsForCommitGithub(conf Conf, flavors map[string]int, flavorDimensions map[int]FlavorDimension) map[string]bool {
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
	var graphQLResponse MergeGraphQLResponseGithub
	err = json.NewDecoder(strings.NewReader(jsonResponse)).Decode(&graphQLResponse)
	if err != nil {
		fail("failed to decode graphql response: %v\n", err)
	}
	if len(graphQLResponse.Data.Repository.Object.PullRequests.Edges) == 0 {
		log.Warnf("No associated pull request found, applying defaults...", err)
		return nil
	}
	var labels = make(map[string]bool)
	for _, label := range graphQLResponse.Data.Repository.Object.PullRequests.Edges[0].Node.Labels.Edges {
		labelName := label.Node.Name
		dimension := flavors[labelName]
		if dimension != 0 {
			flavorDimensions[dimension].SelectedFlavors[labelName] = true
			fmt.Printf("Found label for flavor %s\n", labelName)
		}
		labels[labelName] = true
	}
	return labels
}

func fetchFlavorDimensionsForCommitGitlab(conf Conf, flavors map[string]int, flavorDimensions map[int]FlavorDimension) map[string]bool {
	mergeRequest := fetchMergeRequestForCommitGitlab(conf)

	if mergeRequest == nil {
		log.Warnf("No merge requests found for commit, applying defaults...")
		return nil
	}

	maybeExportDescription(conf, *mergeRequest)

	return processFlavorsGitlab(*mergeRequest, flavors, flavorDimensions)
}

func fetchMergeRequestForCommitGitlab(conf Conf) *MergeRequestGitlab {
	requestBody := `
	{ "query":
		"query {
			project(fullPath: \"$ProjectPath\") {
				mergeRequests(first: 50, state: merged) {
					edges {
						node {
							title,
							titleHtml,
							description,
							descriptionHtml,
							mergeCommitSha,
							labels {
								edges {
		  							node {
										title
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
		"$ProjectPath", conf.ProjectPath,
		"\n", " ",
		"\t", ""}
	err, jsonResponse := graphQLRequest(requestBody, replacements, conf)
	var graphQLResponse MergeGraphQLResponseGitlab
	err = json.NewDecoder(strings.NewReader(jsonResponse)).Decode(&graphQLResponse)
	if err != nil {
		fail("failed to decode graphql response: %v\n", err)
	}
	if len(graphQLResponse.Data.Project.MergeRequests.Edges) == 0 {
		return nil
	}

	mergeRequests := graphQLResponse.Data.Project.MergeRequests.Edges
	for _, mr := range mergeRequests {
		if mr.Node.MergeCommitSha == conf.CommitHash {
			return &mr.Node
		}
	}
	return nil
}

func graphQLRequest(requestBody string, replacements []string, conf Conf) (error, string) {
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

/**
matches labels with environment label specifications "skip_build,dist_*=distribute" and generates environment
variables thereof.

Label specification types:
"some_label": If "some_label" is set in labels, generates an environment variable "some_label" with the content
	"some_label"

"prefix_*": If any label matching "prefix_*" is found, generates an environment variable with the name and content
	of what the placeholder * represents.
	Example: dist_*
		When labels dist_internal and dist_external are set at the PR, this will create the following variables:
		internal=internal
		external=external

"prefix_*=key": If any label matching "prefix_*" is found, sets the environment variable named "key" with the
	content being a comma-separated list of all values that were found represented by the * placeholder.
	Example: dist_*=distribute
		When labels dist_internal and dist_external are set at the PR, this will create the following variable:
		distribute=internal,external

*/
func label2Env(conf Conf, labels map[string]bool) {
	envvars := make(map[string]string)

	for _, envspec := range strings.Split(conf.Labels2Env, ",") {
		parts := strings.Split(envspec, "=")
		pattern := parts[0]
		var labelRegex *regexp.Regexp
		var envKey string
		var envValue string
		if strings.Contains(pattern, "*") {
			pattern = strings.ReplaceAll(pattern, "*", "(.*)")
			labelRegex, _ = regexp.Compile(pattern)
			if (len(parts)) > 1 {
				envKey = parts[1]
			}
		} else {
			labelRegex, _ = regexp.Compile(regexp.QuoteMeta(pattern))
			envKey = parts[0]
			if (len(parts)) > 1 {
				envValue = parts[1]
			}
		}

		for label, _ := range labels {
			matches := labelRegex.FindStringSubmatch(label)
			if len(matches) == 0 {
				continue
			}
			var key string
			var value string
			if len(envValue) > 0 {
				value = envValue
			} else if len(matches) == 1 {
				value = matches[0]
			} else {
				value = matches[1]
			}
			if len(envKey) == 0 {
				key = value
			} else {
				key = envKey
			}
			if len(envvars[key]) != 0 {
				envvars[key] = envvars[key] + "," + value
			} else {
				envvars[key] = value
			}
		}
	}
	for key, value := range envvars {
		err := tools.ExportEnvironmentWithEnvman(key, value)
		if err != nil {
			fmt.Printf("Failed to export environment variable: %s=%s: %v\n", key, value, err)
		}
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
	for i, group := range strings.Split(conf.VariantLabels, "|") {
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
