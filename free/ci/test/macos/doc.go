// Package macos is the macOS leg of the node bootstrap tests (P7-NODE-31, Epic D20): the build-tagged
// tests in this directory install the darwin system layout on the Mac that runs them, through the real
// `nself ci nodes add --system` over a loopback sshd, and prove the separation properties on it. They
// run on GitHub-hosted macos-15 (.github/workflows/ci-node-macos.yml) and on an operator's own Mac when
// the operator starts the leg; the plain `go test ./...` skips the package because every test file
// needs the build tags darwin and macosleg.
package macos
