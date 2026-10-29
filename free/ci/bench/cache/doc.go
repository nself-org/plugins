// Package cache provides performance benchmarks and a harness for measuring
// cache and artifact operations in the CI pipeline. It exposes key derivation,
// content-addressable storage put/open costs, archive pack/unpack throughput,
// and an end-to-end cold/warm restore benchmark that emits perfbench/v1 JSON.
package cache
