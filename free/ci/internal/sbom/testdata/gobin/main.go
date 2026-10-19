// Command gobin is a fixture: a tiny Go program with one third-party
// dependency, built by the sbomtool tests so syft has a Go binary to inventory.
package main

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

func main() {
	out, err := yaml.Marshal(map[string]string{"fixture": "gobin"})
	if err != nil {
		panic(err)
	}
	fmt.Print(string(out))
}
