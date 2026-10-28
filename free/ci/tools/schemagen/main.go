package main

//go:generate go run .

import (
	"log"
	"os"
	"path/filepath"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// main writes deterministic schema bytes from the domain structs: the model, capability and protocol
// schemas here, every other one from the registry (registry.go).
func main() {
	schemas, err := model.Schemas()
	if err != nil {
		log.Fatal(err)
	}
	capability, err := model.CapabilitySchema()
	if err != nil {
		log.Fatal(err)
	}
	schemas["runner-capability.v1.schema.json"] = capability
	runner, err := protocol.Schema()
	if err != nil {
		log.Fatal(err)
	}
	schemas["runner-protocol.v1.schema.json"] = runner
	owned, err := registered()
	if err != nil {
		log.Fatal(err)
	}
	for name, data := range owned {
		schemas[name] = data
	}
	for name, data := range schemas {
		if err := os.WriteFile(filepath.Join("..", "..", "schemas", name), data, 0644); err != nil {
			log.Fatal(err)
		}
	}
}
