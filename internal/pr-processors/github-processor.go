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

type PRGraphQLResponseGithub struct {
	Data struct {
		Repository struct {
			PullRequest MergeRequestGithub
		} `json:"repository"`
	} `json:"data"`
}

type MergeRequestGithub struct {
	Labels struct {
		Edges []struct {
			Node struct {
				Name string `json:"name"`
			} `json:"node"`
		} `json:"edges"`
	} `json:"labels"`
}

type MergeGraphQLResponseGithub struct {
	Data struct {
		Repository struct {
			Object struct {
				PullRequests struct {
					Edges []struct {
						Node MergeRequestGithub
					} `json:"edges"`
				} `json:"associatedPullRequests"`
			} `json:"object"`
		} `json:"repository"`
	} `json:"data"`
}

type GithubProcessor struct {
	conf Conf
}

func NewGithubProcessor(conf Conf) GithubProcessor {
	return GithubProcessor{conf: conf}
}

func (g GithubProcessor) getConf() Conf {
	return g.conf
}

func (g GithubProcessor) processLabelsForPR(flavorDimensions []FlavorDimension) map[string]bool {
	mergeRequest := fetchMergeRequestForPRGithub(g.conf)

	if mergeRequest == nil {
		log.Warnf("Merge request not found, applying defaults...")
		return nil
	}

	// maybeExportDescription(conf, *mergeRequest)

	return selectFlavorsForMergeRequestGithub(mergeRequest, flavorDimensions)
}

func (g GithubProcessor) processLabelsForCommit(flavorDimensions []FlavorDimension) map[string]bool {
	mergeRequest := fetchMergeRequestForCommitGithub(g.conf)

	if mergeRequest == nil {
		log.Warnf("No merge requests found for commit, applying defaults...")
		return nil
	}

	//	maybeExportDescription(conf, *mergeRequest)

	return selectFlavorsForMergeRequestGithub(mergeRequest, flavorDimensions)
}

func fetchMergeRequestForPRGithub(conf Conf) *MergeRequestGithub {
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
	err, jsonResponse := graphql.Request(requestBody, replacements, conf)
	var graphQLResponse PRGraphQLResponseGithub
	err = json.NewDecoder(strings.NewReader(jsonResponse)).Decode(&graphQLResponse)
	if err != nil {
		Fail("failed to decode graphql response: %v\n", err)
	}
	return &graphQLResponse.Data.Repository.PullRequest
}

func selectFlavorsForMergeRequestGithub(mergeRequest *MergeRequestGithub, flavorDimensions []FlavorDimension) map[string]bool {
	mrLabels := mergeRequest.Labels.Edges
	if len(mrLabels) == 0 {
		log.Warnf("No labels found, applying defaults...")
		return nil
	}
	var labels = make(map[string]bool)
	for _, label := range mrLabels {
		labels[label.Node.Name] = true
	}

	if len(labels) == 0 {
		log.Warnf("No labels found, applying defaults...")
		return nil
	}
	fmt.Printf("Found labels: %s\n", strings.Join(Keys(labels), ", "))

	SelectFlavorsFromLabels(labels, flavorDimensions)

	return labels
}
func fetchMergeRequestForCommitGithub(conf Conf) *MergeRequestGithub {
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
	err, jsonResponse := graphql.Request(requestBody, replacements, conf)
	var graphQLResponse MergeGraphQLResponseGithub
	err = json.NewDecoder(strings.NewReader(jsonResponse)).Decode(&graphQLResponse)
	if err != nil {
		Fail("failed to decode graphql response: %v\n", err)
	}
	if len(graphQLResponse.Data.Repository.Object.PullRequests.Edges) == 0 {
		log.Warnf("No associated pull request found, applying defaults...", err)
		return nil
	}
	return &graphQLResponse.Data.Repository.Object.PullRequests.Edges[0].Node
}
