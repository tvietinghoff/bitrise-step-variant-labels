package main

import (
	"encoding/json"
	"fmt"
	"github.com/bitrise-io/go-utils/log"
	"regexp"
	"strings"
)

type PRGraphQLResponseGitlab struct {
	Data struct {
		Project struct {
			MergeRequest MergeRequestGitlab `json:"mergeRequest"`
		} `json:"project"`
	} `json:"data"`
}

type CommitDetails struct {
	Id      string `json:"id"`
	Title   string `json:"title"`
	Message string `json:"message"`
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

type GitlabProcessor struct {
	conf conf
}

func NewGitlabProcessor(conf conf) GitlabProcessor {
	return GitlabProcessor{conf: conf}
}

func (g GitlabProcessor) getConf() conf {
	return g.conf
}

func (g GitlabProcessor) processLabelsForPR(flavorDimensions []flavorDimension) map[string]bool {
	mergeRequest := fetchMergeRequestForPRGitlab(g.conf)

	if mergeRequest == nil {
		log.Warnf("Merge request not found, applying defaults...")
		return nil
	}

	maybeExportDescription(g.conf, *mergeRequest)

	labels := extractLabels(mergeRequest)

	return selectFlavorsForLabels(labels, flavorDimensions)
}

func (g GitlabProcessor) processLabelsForCommit(flavorDimensions []flavorDimension) map[string]bool {
	mergeRequest := fetchMergeRequestForCommitGitlab(g.conf)

	var labels map[string]bool

	if mergeRequest == nil {
		if len(g.conf.ProjectId) == 0 {
			log.Warnf("No merge requests found for commit. Please configure the project ID if you want to specify build labels in the commit message...")
		} else {
			log.Warnf("No merge requests found for commit, checking commit message...")
			commitDetails := fetchCommitDetails(g.conf.CommitHash, g.conf)
			labels = extractLabelsFromCommit(commitDetails)
		}
		if len(labels) == 0 {
			log.Warnf("No labels found, applying defaults...")
		}
	} else {
		labels = extractLabels(mergeRequest)
		maybeExportDescription(g.conf, *mergeRequest)
	}

	return selectFlavorsForLabels(labels, flavorDimensions)
}

func selectFlavorsForLabels(labels map[string]bool, flavorDimensions []flavorDimension) map[string]bool {

	if len(labels) == 0 {
		log.Warnf("No labels found, applying defaults...")
		return nil
	}

	fmt.Printf("Found labels: %s\n", strings.Join(keys(labels), ", "))

	selectFlavorsFromLabels(labels, flavorDimensions)

	return labels
}

func extractLabels(mergeRequest *MergeRequestGitlab) map[string]bool {
	mrLabels := mergeRequest.Labels.Edges
	if len(mrLabels) == 0 {
		log.Warnf("No labels found, applying defaults...")
		return map[string]bool{}
	}
	var labels = make(map[string]bool)
	for _, label := range mrLabels {
		labelName := label.Node.Title
		labels[labelName] = true
	}
	return labels
}

func extractLabelsFromCommit(commitDetails CommitDetails) map[string]bool {
	var reLabels = regexp.MustCompile(`\[labels:([^\]]*)\]`)
	var reSeparators = regexp.MustCompile(`[,\s]+`)

	var labels = make(map[string]bool)
	for _, submatch := range reLabels.FindAllStringSubmatch(commitDetails.Message, -1) {
		labelList := submatch[1]
		for _, label := range reSeparators.Split(labelList, -1) {
			if len(label) > 0 {
				labels[label] = true
			}
		}
	}

	if len(labels) == 0 {
		log.Debugf("No labels found in commit message")
		return map[string]bool{}
	}
	return labels
}

func fetchMergeRequestForCommitGitlab(conf conf) *MergeRequestGitlab {
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
	err, jsonResponse := GraphQlRequest(requestBody, replacements, conf)
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

func fetchMergeRequestForPRGitlab(conf conf) *MergeRequestGitlab {
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
	err, jsonResponse := GraphQlRequest(requestBody, replacements, conf)
	var graphQLResponse PRGraphQLResponseGitlab
	err = json.NewDecoder(strings.NewReader(jsonResponse)).Decode(&graphQLResponse)
	if err != nil {
		fail("failed to decode graphql response: %v\n", err)
	}
	return &graphQLResponse.Data.Project.MergeRequest
}

func fetchCommitDetails(commitSha string, conf conf) CommitDetails {
	err, jsonResponse := ApiRequest(
		strings.NewReplacer(":sha", commitSha, ":projectId", conf.ProjectId).
			Replace("https://gitlab.com/api/v4/projects/:projectId/repository/commits/:sha"),
		conf, "GET", "")

	if err != nil {
		fail("Failed to get commit details: %v\n", err)
	}
	var commitApiResponse CommitDetails
	err = json.NewDecoder(strings.NewReader(jsonResponse)).Decode(&commitApiResponse)
	if err != nil {
		fail("Failed to decode commit details: %v\n", err)
	}
	return commitApiResponse
}
