package buildvariants

import (
	. "bitrise-step-variant-labels/internal/common"
	"fmt"
	"regexp"
	"strings"
)

type FlavorDimension struct {
	Index           int
	LabelMatcher    string
	LabelRegex      *regexp.Regexp
	FlavorNames     map[string]string
	FlavorNameSlice []string
	DefaultFlavor   string
	SelectedFlavors map[string]bool
}

func SelectFlavorsFromLabels(labels map[string]bool, flavorDimensions map[int]FlavorDimension) {
	for label := range labels {
		selectFlavor(label, flavorDimensions)
	}
}

func selectFlavor(label string, flavorDimensions map[int]FlavorDimension) {
	for _, flavorDimension := range flavorDimensions {
		if flavorDimension.LabelRegex == nil {
			flavorDimension.LabelRegex, _ = regexp.Compile(flavorDimension.LabelMatcher)
		}
		matches := flavorDimension.LabelRegex.FindStringSubmatch(label)
		if len(matches) < 2 {
			continue
		}
		if flavorDimension.FlavorNameSlice == nil {
			flavorNames := make([]string, 0, len(flavorDimension.FlavorNames))
			for _, flavor := range flavorDimension.FlavorNames {
				flavorNames = append(flavorNames, flavor)
			}
			flavorDimension.FlavorNameSlice = flavorNames
		}
		fmt.Printf("Label %s is matching %s\n", label, flavorDimension.LabelMatcher)
		matches = matches[1:]
		for index, submatch := range matches {
			if len(submatch) > 0 {
				var flavor string
				if len(flavorDimension.FlavorNameSlice) > 0 {
					flavor = flavorDimension.FlavorNameSlice[index]
				} else {
					flavor = submatch
				}
				fmt.Printf("Selecing flavor %s\n", flavor)
				flavorDimension.SelectedFlavors[flavor] = true
			}
		}
	}
}

func GetFlavorDimensions(conf Conf) map[int]FlavorDimension {
	flavorDimensions := make(map[int]FlavorDimension)
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
				flavorDimension.LabelMatcher = ""
				flavorDimension.FlavorNames = make(map[string]string)
				flavorDimension.SelectedFlavors = make(map[string]bool)
			}

			// generate or add a matcher pattern for the label
			if strings.Contains(label, "*") {
				if len(flavorDimension.LabelMatcher) > 0 {
					Fail("Cannot mix and match verbatim labels and label patterns:\n%s\n%s", flavorDimension.LabelMatcher, label)
				}
				// label spec contains a wildcard, use the spec itself as a pattern
				flavorDimension.LabelMatcher = strings.ReplaceAll(label, "*", "(.*)")
			} else {
				// label spec is verbatim, build a list of alternatives from the provided labels
				if len(flavorDimension.LabelMatcher) > 0 {
					if strings.Contains(flavorDimension.LabelMatcher, "*") {
						Fail("Cannot mix and match verbatim labels and label patterns:\n%s\n%s",
							flavorDimension.LabelMatcher, label)
					}
					flavorDimension.LabelMatcher += "|"
				}
				flavorDimension.LabelMatcher += "(^" + label + "$)"
				flavorDimension.FlavorNames[label] = flavorName
			}

			if isDefault {
				flavorDimension.DefaultFlavor = label
			}
			flavorDimensions[index] = flavorDimension
		}
	}
	return flavorDimensions
}
