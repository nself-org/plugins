package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// main writes deterministic schema bytes from the domain structs.
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
	for name, data := range schemas {
		if err := os.WriteFile(filepath.Join("..", "..", "schemas", name), data, 0644); err != nil {
			log.Fatal(err)
		}
	}
}
