package buildvariants

import (
	. "bitrise-step-variant-labels/internal/common"
	"fmt"
	"regexp"
	"strings"
)

type FlavorDimension struct {
	LabelMatcher    string
	LabelRegex      *regexp.Regexp
	FlavorNames     map[string]string
	FlavorNameSlice []string
	DefaultFlavor   string
	SelectedFlavors map[string]bool
}

func SelectFlavorsFromLabels(labels map[string]bool, flavorDimensions []FlavorDimension) {
	for label := range labels {
		selectFlavor(label, flavorDimensions)
	}
}

func selectFlavor(label string, flavorDimensions []FlavorDimension) {
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

func GetFlavorDimensions(conf Conf) []FlavorDimension {
	dimensionSpecs := strings.Split(conf.VariantLabels, "|")
	flavorDimensions := make([]FlavorDimension, 0, len(dimensionSpecs))
	for index, group := range dimensionSpecs {
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

			if len(flavorDimensions) <= index {
				flavorDimensions = append(flavorDimensions, FlavorDimension{
					LabelMatcher:    "",
					FlavorNames:     make(map[string]string),
					SelectedFlavors: make(map[string]bool),
				})
			}
			flavorDimension := &flavorDimensions[index]
			if isDefault {
				flavorDimension.DefaultFlavor = label
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
						if isDefault {
							continue
						}
						Fail("Cannot mix and match verbatim labels and label patterns:\n%s\n%s",
							flavorDimension.LabelMatcher, label)
					} else {
						flavorDimension.LabelMatcher += "|"
					}
				}
				flavorDimension.LabelMatcher += "(^" + label + "$)"
				flavorDimension.FlavorNames[label] = flavorName
			}

		}
	}
	return flavorDimensions
}
