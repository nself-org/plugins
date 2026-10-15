package main

import "testing"

func TestDocsRouteIncluded(t *testing.T) {
	m := loadRoutes("../../testdata/fixtures/tls-full/routes.json")
	for _, rt := range m.Routes {
		if rt.ID == "core:api-docs" {
			if len(rt.Locations) != 2 || rt.Locations[0].StaticFile == nil || rt.Locations[0].HeadersAdd["Cache-Control"] != "no-store" {
				t.Fatalf("docs locations missing: %+v", rt)
			}
			return
		}
	}
	t.Fatal("docs route excluded from parity")
}
