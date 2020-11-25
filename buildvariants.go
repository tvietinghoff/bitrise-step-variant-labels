package main

import (
	"fmt"
	"regexp"
	"strings"
)

type flavorDimension struct {
	labelMatcher    string
	labelRegex      *regexp.Regexp
	flavorNames     map[string]string
	flavorNameSlice []string
	defaultFlavor   string
	selectedFlavors map[string]bool
}

func selectFlavorsFromLabels(labels map[string]bool, flavorDimensions []flavorDimension) {
	for label := range labels {
		selectFlavor(label, flavorDimensions)
	}
}

func selectFlavor(label string, flavorDimensions []flavorDimension) {
	for _, flavorDimension := range flavorDimensions {
		if flavorDimension.labelRegex == nil {
			flavorDimension.labelRegex, _ = regexp.Compile(flavorDimension.labelMatcher)
		}
		matches := flavorDimension.labelRegex.FindStringSubmatch(label)
		if len(matches) < 2 {
			continue
		}
		if flavorDimension.flavorNameSlice == nil {
			flavorNames := make([]string, 0, len(flavorDimension.flavorNames))
			for _, flavor := range flavorDimension.flavorNames {
				flavorNames = append(flavorNames, flavor)
			}
			flavorDimension.flavorNameSlice = flavorNames
		}
		fmt.Printf("Label %s is matching %s\n", label, flavorDimension.labelMatcher)
		matches = matches[1:]
		for index, submatch := range matches {
			if len(submatch) > 0 {
				var flavor string
				if len(flavorDimension.flavorNameSlice) > 0 {
					flavor = flavorDimension.flavorNameSlice[index]
				} else {
					flavor = submatch
				}
				fmt.Printf("Selecing flavor %s\n", flavor)
				flavorDimension.selectedFlavors[flavor] = true
			}
		}
	}
}

func getFlavorDimensions(conf conf) []flavorDimension {
	dimensionSpecs := strings.Split(conf.VariantLabels, "|")
	flavorDimensions := make([]flavorDimension, 0, len(dimensionSpecs))
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
				flavorDimensions = append(flavorDimensions, flavorDimension{
					labelMatcher:    "",
					flavorNames:     make(map[string]string),
					selectedFlavors: make(map[string]bool),
				})
			}
			flavorDimension := &flavorDimensions[index]
			if isDefault {
				flavorDimension.defaultFlavor = label
			}

			// generate or add a matcher pattern for the label
			if strings.Contains(label, "*") {
				if len(flavorDimension.labelMatcher) > 0 {
					fail("Cannot mix and match verbatim labels and label patterns:\n%s\n%s", flavorDimension.labelMatcher, label)
				}
				// label spec contains a wildcard, use the spec itself as a pattern
				flavorDimension.labelMatcher = strings.ReplaceAll(label, "*", "(.*)")
			} else {
				// label spec is verbatim, build a list of alternatives from the provided labels
				if len(flavorDimension.labelMatcher) > 0 {
					if strings.Contains(flavorDimension.labelMatcher, "*") {
						if isDefault {
							continue
						}
						fail("Cannot mix and match verbatim labels and label patterns:\n%s\n%s",
							flavorDimension.labelMatcher, label)
					} else {
						flavorDimension.labelMatcher += "|"
					}
				}
				flavorDimension.labelMatcher += "(^" + label + "$)"
				flavorDimension.flavorNames[label] = flavorName
			}

		}
	}
	return flavorDimensions
}
