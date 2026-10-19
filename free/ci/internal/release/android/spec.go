// Package android is the V15 Android release lane (Epic CACHE D18,
// cap:ci.android-release): the release job and pipeline `nself ci build
// --artifact android` runs, the SBOM step of its APKs and the explicit
// `gh release upload` publish step.
package android

// Purpose: describe the Android lane as data the V15 engine already runs: a
// `kind: release` job named `android` and, when the project's config has none,
// the pipeline `android-release` that wraps it.
// Inputs:  the directory of the Android project (workdir relative to the
// repository root), the build timeout and the names of the project's required
// jobs.
// Outputs: a model.Job and the pipeline config that holds it.
// Constraints: the keystore reaches the job only as the four release/ANDROID_*
// secret refs (the personal-node compat source of P7-TRUST-02 fills them from
// the operator's environment, on the operator's own machine, and refuses every
// other node, E677); the job decodes the keystore into app/release.keystore
// inside its own workspace and removes it when the script ends, so no
// keystore file is left behind; nothing here reads the environment, the disk
// or the network.

import (
	"strconv"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Names of the lane.
const (
	JobName      = "android"
	PipelineName = "android-release"
	// ArtifactName is the record-name prefix of the APKs (android/<file>.apk).
	ArtifactName = "android"
	// APKGlob selects the signed release APKs, relative to the job workdir.
	APKGlob = "app/build/outputs/apk/release/*.apk"
	// KeystorePath is where the job decodes the keystore, relative to the workdir.
	KeystorePath = "app/release.keystore"
	// SBOMDir is where the SBOM step writes the documents, relative to the workdir.
	SBOMDir = ".nself-ci/sbom"
)

// SecretRefs are the release-scoped secrets the job declares.
var SecretRefs = []string{
	"release/ANDROID_KEYSTORE_BASE64",
	"release/ANDROID_KEYSTORE_PASSWORD",
	"release/ANDROID_KEY_ALIAS",
	"release/ANDROID_KEY_PASSWORD",
}

// script decodes the keystore from its env var, builds, and removes the keystore
// whatever the outcome (the trap runs on exit and on a signal).
const script = `set -eu
umask 077
trap 'rm -f ` + KeystorePath + `' EXIT INT TERM
mkdir -p app
printf '%s' "$ANDROID_KEYSTORE_BASE64" | tr -d '[:space:]' | { base64 -d 2>/dev/null || base64 -D; } > ` + KeystorePath + `
test -s ` + KeystorePath + `
./gradlew assembleRelease`

// Job is the synthesized release job: workdir is the Android project relative
// to the repository root, deps the project's required jobs.
func Job(workdir string, deps []string, timeout time.Duration) model.Job {
	required, idempotent := true, false
	secs := int(timeout / time.Second)
	if secs < 1 {
		secs = 1
	}
	return model.Job{
		Kind:       "release",
		Run:        []string{"sh", "-c", script},
		Workdir:    workdir,
		Deps:       append([]string(nil), deps...),
		Required:   &required,
		Idempotent: &idempotent,
		Timeout:    strconv.Itoa(secs) + "s",
		Isolation:  "process",
		Secrets:    append([]string(nil), SecretRefs...),
		Artifacts: []model.ArtifactDecl{
			{Path: APKGlob, Name: ArtifactName, Kind: "package"},
			{Path: SBOMDir + "/", Kind: "sbom"},
		},
	}
}
