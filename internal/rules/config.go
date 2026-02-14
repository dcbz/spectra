package rules

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadFromFile reads a YAML rule configuration and compiles it.
func LoadFromFile(path string) (RuleSet, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return RuleSet{}, err
	}

	var rf ruleFile
	if err := yaml.Unmarshal(content, &rf); err != nil {
		return RuleSet{}, fmt.Errorf("parse rules: %w", err)
	}

	return Compile(rf.Rules)
}
