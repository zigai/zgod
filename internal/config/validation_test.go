package config

import (
	"os"
	"testing"
)

func TestInvalidValuesDoNotCreateDefaults(t *testing.T) {
	for _, assignment := range []string{
		"display.startup_limit=-1",
		"display.time_format=\"invalid\"",
		"display.duration_format=\"invalid\"",
		"filters.max_command_length=-1",
		"filters.command_regex=[\"[\"]",
		"filters.directory_regex=[\"[\"]",
		"filters.directory_glob=[\"[\"]",
	} {
		t.Run(assignment, func(t *testing.T) {
			path := setTestHomes(t, t.TempDir())

			opts := LoadOptions{Path: "", NoConfig: false, NoCreate: false, Overrides: []string{assignment}}
			if _, err := LoadWithOptions(opts); err == nil {
				t.Fatal("invalid value accepted")
			}

			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("invalid value created config: %v", err)
			}
		})
	}
}
