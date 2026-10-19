package main

// Purpose: the show, get and prune subcommands of `nself ci artifacts`
// (P7-CACHE-03; list, the registry and the --cache slot are in artifacts.go).
// Inputs:  argv after the subcommand name; the environment artifacts.go documents.
// Outputs: key/value lines or one v1 JSON envelope; exit 0, 1 or 2.
// Constraints: get writes only into a fresh empty directory that
// authz.ArtifactGetDest accepted; prune never touches release-kind records
// without --release; not found and not visible print the same E713.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
)

func artifactsShow(args []string) int {
	fs, pos, err := parseArtifacts("show", args, nil)
	if err != nil || len(pos) != 1 {
		fmt.Fprintln(os.Stderr, "usage: nself ci artifacts show <id> [--json]")
		return 1
	}
	env, err := openArtifacts(fs)
	if err != nil {
		return artifactsExit(err, "ci artifacts show", false)
	}
	defer env.close()
	r, err := env.reader.Show(context.Background(), env.viewer, pos[0])
	if err != nil {
		return artifactsExit(err, "ci artifacts show", env.json)
	}
	if env.json {
		return writeArtifactsJSON("ci artifacts show", r)
	}
	exp := "never"
	if r.ExpiresAt != nil {
		exp = r.ExpiresAt.Format(time.RFC3339)
	}
	fmt.Printf("id %s\nname %s\nkind %s\nsha256 %s\nsize %d\nproject %s\nrun %s\njob %s\nattempt %s\nrevision %s\nplatform %s\nrunner %s\nisolation %s\nproducer %s\nvisibility %s\ncreated %s\nexpires %s\n",
		r.ID, r.Name, r.Kind, r.SHA256, r.Size, r.Project, r.Run, r.Job, r.Attempt, r.Revision, r.Platform, r.RunnerID, r.Isolation, r.Producer, r.Visibility, r.CreatedAt.Format(time.RFC3339), exp)
	if r.Image != nil {
		fmt.Printf("image index %s\n", r.Image.IndexDigest)
		for _, m := range r.Image.Manifests {
			fmt.Printf("image manifest %s %s\n", m.Platform, m.Digest)
		}
	}
	if r.Missing {
		fmt.Println("status missing: the stored bytes are gone; `get` will fail E712 missing_blob")
	}
	return 0
}

func artifactsGet(args []string) int {
	var dest *string
	fs, pos, err := parseArtifacts("get", args, func(fs *flag.FlagSet) {
		dest = fs.String("dest", "", "fresh empty directory to write into (never the working directory or under .git)")
	})
	if err != nil || len(pos) != 1 || *dest == "" {
		fmt.Fprintln(os.Stderr, "usage: nself ci artifacts get <id> --dest <dir> [--json]")
		return 1
	}
	env, err := openArtifacts(fs)
	if err != nil {
		return artifactsExit(err, "ci artifacts get", false)
	}
	defer env.close()
	cwd, err := os.Getwd()
	if err != nil {
		return artifactsExit(err, "ci artifacts get", env.json)
	}
	path, r, err := env.reader.Get(context.Background(), env.viewer, pos[0], *dest, cwd)
	if err != nil {
		return artifactsExit(err, "ci artifacts get", env.json)
	}
	if env.json {
		return writeArtifactsJSON("ci artifacts get", map[string]any{"path": path, "record": r})
	}
	fmt.Println(path)
	return 0
}

func artifactsPrune(args []string) int {
	var release, dry *bool
	var older *string
	fs, _, err := parseArtifacts("prune", args, func(fs *flag.FlagSet) {
		release = fs.Bool("release", false, "also remove release-kind artifacts (never removed otherwise)")
		older = fs.String("older-than", "", "with --release: remove release artifacts older than this (default 365d)")
		dry = fs.Bool("dry-run", false, "list what would be removed")
	})
	if err != nil {
		return 1
	}
	spec := records.PruneSpec{Now: time.Now(), Release: *release, DryRun: *dry}
	if *older != "" {
		d, _, e := records.RetentionDuration(*older)
		if e != nil || !*release {
			fmt.Fprintln(os.Stderr, "nself ci artifacts prune: --older-than needs --release and a duration like 90d")
			return 1
		}
		spec.OlderThan = d
	}
	env, err := openArtifacts(fs)
	if err != nil {
		return artifactsExit(err, "ci artifacts prune", false)
	}
	defer env.close()
	removed, err := records.Prune(context.Background(), env.st, env.blobs, spec)
	var bytes int64
	for _, r := range removed {
		bytes += r.Size
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		return artifactsExit(err, "ci artifacts prune", env.json)
	}
	if env.json {
		return writeArtifactsJSON("ci artifacts prune", map[string]any{"dry_run": *dry, "release": *release, "removed": orEmpty(removed), "bytes": bytes})
	}
	verb := "removed"
	if *dry {
		verb = "would remove"
	}
	for _, r := range removed {
		fmt.Printf("%s %s %s (%d B)\n", verb, r.ID, r.Name, r.Size)
	}
	fmt.Printf("%s %d record(s), %d B\n", verb, len(removed), bytes)
	return 0
}
