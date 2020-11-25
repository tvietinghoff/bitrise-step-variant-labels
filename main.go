package main

import (
	"fmt"
	"github.com/bitrise-io/go-steputils/tools"
	"github.com/bitrise-tools/go-steputils/stepconf"
	"os"
	"regexp"
	"strings"
)

func main() {
	var conf conf

	if err := stepconf.Parse(&conf); err != nil {
		fail("step config failed: %v\n", err)
	}
	printconf := conf
	printconf.AuthToken = "***"

	stepconf.Print(printconf)

	if len(conf.Provider) == 0 {
		conf.Provider = "github"
	}

	flavorDimensions := getFlavorDimensions(conf)
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

	var processor prLabelProcessor
	if conf.Provider == "github" {
		processor = NewGithubProcessor(conf)
	} else if conf.Provider == "gitlab" {
		processor = NewGitlabProcessor(conf)
	} else {
		fail("Invalid provider: %v. Allowed are: github, gitlab", conf.Provider)
	}

	labels := processLabels(processor, flavorDimensions)

	label2Env(conf, labels)

	for key, pattern := range variantPatterns {
		generateEnvironmentVariable(key, pattern, flavorDimensions)
	}

	os.Exit(0)
}

func generateEnvironmentVariable(key string, pattern string, flavorDimensions []flavorDimension) {
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
		placeholder := fmt.Sprintf("#%d", index+1)
		selectedFlavors := flavorDimension.selectedFlavors
		if len(selectedFlavors) == 0 {
			selectedFlavors = make(map[string]bool)
			selectedFlavors[flavorDimension.defaultFlavor] = true
			fmt.Printf("No label for flavor dimension %d found, defaulting to %s\n", index+1, flavorDimension.defaultFlavor)
		}
		for flavor := range selectedFlavors {
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
func label2Env(conf conf, labels map[string]bool) {
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

		for label := range labels {
			matches := labelRegex.FindStringSubmatch(label)
			if len(matches) == 0 {
				continue
			}
			fmt.Printf("Found label for envvar: %s\n", label)

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
		fmt.Printf("%s = %s\n", key, value)
		err := tools.ExportEnvironmentWithEnvman(key, value)
		if err != nil {
			fmt.Printf("Failed to export environment variable: %s=%s: %v\n", key, value, err)
		}
	}
}
