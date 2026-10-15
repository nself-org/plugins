# Go CI coverage

`.github/workflows/build-test.yml` discovers Go modules recursively under
`free/`, `sdk/`, and `scripts/`, excluding vendored modules. Each discovered
module runs `go build -o /dev/null ./...`, `go vet ./...`, and `go test ./...`.
A failed module fails the job; the loop continues to report other failures.
Modules under `tests/conformance/` are fixtures, outside this discovery scope.

The workflow regression tests execute its shell against fixture modules with a
recording Go command. They check nested plugin discovery, SDK/tool coverage,
vendor and fixture exclusion, and propagation of build, vet, and test failures.
Run them with:

```sh
python3 -m unittest discover -s scripts/ci/tests -p test_go_build_workflow.py -v
```
