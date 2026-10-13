package config

import (
	"maps"
	"slices"
)

// Keys is every key the config accepts, sorted, for the tests.
func Keys() []string { return slices.Sorted(maps.Keys(keys)) }
