package client

import _ "embed"

//go:embed config.example.yaml
var exampleConfigYAML string

// ExampleConfigYAML returns a canonical example client configuration
// in YAML format. This can be used by the CLI to show a reference
// configuration to end users.
func ExampleConfigYAML() string {
	return exampleConfigYAML
}
