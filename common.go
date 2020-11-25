package main

import (
	"github.com/bitrise-io/go-utils/log"
	"os"
)

func fail(message string, args ...interface{}) {
	log.Errorf(message, args...)
	os.Exit(1)
}

func keys(stringKeyMap map[string]bool) []string {
	keys := make([]string, 0, len(stringKeyMap))
	for k := range stringKeyMap {
		keys = append(keys, k)
	}
	return keys
}
