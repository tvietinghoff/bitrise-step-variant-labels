package pr_processors

import (
	"bitrise-step-variant-labels/internal/buildvariants"
	"bitrise-step-variant-labels/internal/common"
	"github.com/bitrise-io/go-utils/log"
	"io/ioutil"
	"path/filepath"
	"strings"
)

type PrLabelProcessor interface {
	getConf() common.Conf
	processLabelsForPR(dimensions []buildvariants.FlavorDimension) map[string]bool
	processLabelsForCommit(dimensions []buildvariants.FlavorDimension) map[string]bool
}

func ProcessLabels(p PrLabelProcessor, flavorDimensions []buildvariants.FlavorDimension) map[string]bool {
	var labels map[string]bool
	conf := p.getConf()
	if conf.PullRequest != 0 {
		labels = p.processLabelsForPR(flavorDimensions)
	} else if conf.CommitHash != "" {
		labels = p.processLabelsForCommit(flavorDimensions)
	} else {
		log.Warnf("Neither commit_hash nor pull_request given. Building defaults only.")
		for index, dimension := range flavorDimensions {
			if dimension.DefaultFlavor == "" {
				common.Fail("Missing default for flavor dimension %d, aborting...", index)
			}
		}
		labels = nil
	}
	return labels
}

func maybeExportDescription(conf common.Conf, mergeRequest MergeRequestGitlab) {
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
			err := ioutil.WriteFile(path, []byte(description), 0644)
			if err != nil {
				log.Warnf("Writing description failed: File: %s", path)
			}
		}
	}
	if len(ext) == 0 || strings.ToLower(ext) == ".html" {
		if len(html) == 0 {
			log.Warnf("HTML description not available, but export was requested")
		} else {
			path := strings.TrimSuffix(conf.ExportDescription, ".html") + ".html"
			err := ioutil.WriteFile(path, []byte(html), 0644)
			if err != nil {
				log.Warnf("Writing description failed: File: %s", path)
			}
		}
	}
}
