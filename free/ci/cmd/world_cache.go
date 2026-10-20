package main

// Purpose: register the `cache` world contributor (Epic CACHE D8). It fills sched.World.Cache with
// the bytes each runner would fetch for each job's cache entries, so placement prefers runners
// that already hold them.
// Inputs:  the source the run or plan code binds with locality.WithSource (jobs with their detector
// facts, and the read-only store view); none bound means no facts.
// Outputs: contributor "cache" owning the Cache field of the world.
// Constraints: read-only, no network; registration only (no subcommand, no flag, no code).

import (
	"github.com/nself-org/plugins/free/ci/internal/cache/locality"
	"github.com/nself-org/plugins/free/ci/internal/world"
)

func init() {
	if err := world.Register(world.Contributor{Name: locality.Name, Field: locality.Field, Fn: locality.Contribute}); err != nil {
		panic("world_cache: " + err.Error())
	}
}
