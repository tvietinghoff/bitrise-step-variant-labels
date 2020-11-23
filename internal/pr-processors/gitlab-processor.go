package pr_processors

import (
	. "bitrise-step-variant-labels/internal/buildvariants"
	. "bitrise-step-variant-labels/internal/common"
	"bitrise-step-variant-labels/internal/graphql"
	"encoding/json"
	"fmt"
	"github.com/bitrise-io/go-utils/log"
	"strings"
)

type PRGraphQLResponseGitlab struct {
	Data struct {
		Project struct {
			MergeRequest MergeRequestGitlab `json:"mergeRequest"`
		} `json:"project"`
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

type GitlabProcessor struct {
	conf Conf
}

func NewGitlabProcessor(conf Conf) GitlabProcessor {
	return GitlabProcessor{conf: conf}
}

func (g GitlabProcessor) getConf() Conf {
	return g.conf
}

func (g GitlabProcessor) processLabelsForPR(flavorDimensions map[int]FlavorDimension) map[string]bool {
	mergeRequest := fetchMergeRequestForPRGitlab(g.conf)

	if mergeRequest == nil {
		log.Warnf("Merge request not found, applying defaults...")
		return nil
	}

	maybeExportDescription(g.conf, *mergeRequest)

	return selectFlavorsForMergeRequestGitlab(mergeRequest, flavorDimensions)
}

func (g GitlabProcessor) processLabelsForCommit(flavorDimensions map[int]FlavorDimension) map[string]bool {
	mergeRequest := fetchMergeRequestForCommitGitlab(g.conf)

	if mergeRequest == nil {
		log.Warnf("No merge requests found for commit, applying defaults...")
		return nil
	}

	maybeExportDescription(g.conf, *mergeRequest)

	return selectFlavorsForMergeRequestGitlab(mergeRequest, flavorDimensions)
}

func selectFlavorsForMergeRequestGitlab(mergeRequest *MergeRequestGitlab, flavorDimensions map[int]FlavorDimension) map[string]bool {
	mrLabels := mergeRequest.Labels.Edges
	if len(mrLabels) == 0 {
		log.Warnf("No labels found, applying defaults...")
		return nil
	}
	var labels = make(map[string]bool)
	for _, label := range mrLabels {
		labelName := label.Node.Title
		labels[labelName] = true
	}

	if len(labels) == 0 {
		log.Warnf("No labels found, applying defaults...")
		return nil
	}

	fmt.Printf("Found labels: %s\n", strings.Join(Keys(labels), ", "))

	SelectFlavorsFromLabels(labels, flavorDimensions)

	return labels
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
	err, jsonResponse := graphql.Request(requestBody, replacements, conf)
	var graphQLResponse MergeGraphQLResponseGitlab
	err = json.NewDecoder(strings.NewReader(jsonResponse)).Decode(&graphQLResponse)
	if err != nil {
		Fail("failed to decode graphql response: %v\n", err)
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
	err, jsonResponse := graphql.Request(requestBody, replacements, conf)
	var graphQLResponse PRGraphQLResponseGitlab
	err = json.NewDecoder(strings.NewReader(jsonResponse)).Decode(&graphQLResponse)
	if err != nil {
		Fail("failed to decode graphql response: %v\n", err)
	}
	return &graphQLResponse.Data.Project.MergeRequest
}
