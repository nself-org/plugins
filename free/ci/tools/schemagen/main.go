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
	for name, data := range schemas {
		if err := os.WriteFile(filepath.Join("..", "..", "schemas", name), data, 0644); err != nil {
			log.Fatal(err)
		}
	}
}
