package bench

// Purpose: the Go benchmarks of the in-process scenarios (`go test -bench`).
// They call the same functions cibench times, so a benchmark number and a
// result-JSON number measure the same code.

import (
	"context"
	"testing"
)

func benchInproc(b *testing.B) *Inproc {
	b.Helper()
	dir := b.TempDir()
	if err := Materialize(context.Background(), Tiny, dir+"/repo"); err != nil {
		b.Fatal(err)
	}
	p, err := PrepareInproc(dir+"/repo", dir+"/scratch")
	if err != nil {
		b.Fatal(err)
	}
	return p
}

func BenchmarkDetect(b *testing.B) {
	p := benchInproc(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := p.Detect(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPlan(b *testing.B) {
	p := benchInproc(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := p.Plan(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStoreOpen(b *testing.B) {
	p := benchInproc(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := p.StoreOpen(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkExecOverhead(b *testing.B) {
	p := benchInproc(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := p.ExecOverhead(context.Background()); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRawSpawn(b *testing.B) {
	p := benchInproc(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := p.RawSpawn(context.Background()); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCapacityProbe(b *testing.B) {
	for i := 0; i < b.N; i++ {
		CapacityProbe()
	}
}
