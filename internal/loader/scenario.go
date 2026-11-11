package loader

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadScenario reads and validates a YAML scenario file. Unknown keys are errors so typos don't pass silently.
func LoadScenario(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	return ParseScenario(b)
}

func ParseScenario(b []byte) (Config, error) {
	var c Config
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return Config{}, fmt.Errorf("parse scenario: %w", err)
	}
	return c, c.Validate()
}

func MarshalScenario(c Config) ([]byte, error) { return yaml.Marshal(c) }
