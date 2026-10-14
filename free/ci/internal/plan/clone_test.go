package plan

import (
	"reflect"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// fill sets every field of v to a non-zero value, recursively, so the alias
// walk sees every pointer, slice and map a Job can hold (a field added to
// model.Job later is covered without editing this test).
func fill(v reflect.Value) {
	switch v.Kind() {
	case reflect.String:
		v.SetString("x")
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int64:
		v.SetInt(3)
	case reflect.Pointer:
		v.Set(reflect.New(v.Type().Elem()))
		fill(v.Elem())
	case reflect.Slice:
		v.Set(reflect.MakeSlice(v.Type(), 1, 1))
		fill(v.Index(0))
	case reflect.Map:
		v.Set(reflect.MakeMap(v.Type()))
		k, e := reflect.New(v.Type().Key()).Elem(), reflect.New(v.Type().Elem()).Elem()
		fill(k)
		fill(e)
		v.SetMapIndex(k, e)
	case reflect.Interface:
		v.Set(reflect.ValueOf([]any{"x"}))
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Field(i).CanSet() {
				fill(v.Field(i))
			}
		}
	}
}

// shared lists the paths where a and b hold the same pointer, slice or map.
func shared(a, b reflect.Value, path string, out *[]string) {
	switch a.Kind() {
	case reflect.Pointer:
		if a.IsNil() || b.IsNil() {
			return
		}
		if a.Pointer() == b.Pointer() {
			*out = append(*out, path)
			return
		}
		shared(a.Elem(), b.Elem(), path, out)
	case reflect.Slice:
		if a.Len() == 0 || b.Len() == 0 {
			return
		}
		if a.Pointer() == b.Pointer() {
			*out = append(*out, path)
			return
		}
		for i := 0; i < a.Len() && i < b.Len(); i++ {
			shared(a.Index(i), b.Index(i), path+"[]", out)
		}
	case reflect.Map:
		if a.Len() == 0 || b.Len() == 0 {
			return
		}
		if a.Pointer() == b.Pointer() {
			*out = append(*out, path)
			return
		}
		for _, k := range a.MapKeys() {
			if bv := b.MapIndex(k); bv.IsValid() {
				shared(a.MapIndex(k), bv, path+"{}", out)
			}
		}
	case reflect.Interface:
		if !a.IsNil() && !b.IsNil() {
			shared(a.Elem(), b.Elem(), path, out)
		}
	case reflect.Struct:
		for i := 0; i < a.NumField(); i++ {
			shared(a.Field(i), b.Field(i), path+"."+a.Type().Field(i).Name, out)
		}
	}
}

func TestCloneJobSharesNothing(t *testing.T) {
	var j model.Job
	fill(reflect.ValueOf(&j).Elem())
	c := cloneJob(j)
	var out []string
	shared(reflect.ValueOf(j), reflect.ValueOf(c), "Job", &out)
	if len(out) > 0 {
		t.Errorf("cloneJob aliases %v", out)
	}
	if !reflect.DeepEqual(j, c) {
		t.Error("clone is not equal to the original")
	}
}

func mutableMatrixJob() model.Job {
	j := model.Job{Kind: "build", Run: []string{"make"}, Matrix: &model.JobMatrix{Platform: []any{"linux/amd64", "darwin/arm64"}},
		Env: map[string]string{"A": "1"}, Secrets: []string{"s"}, NeedsServices: []string{"db"},
		When:      &model.WhenConfig{Branches: []string{"main"}, Paths: []string{"a/**"}},
		Container: &model.ContainerConfig{Image: "img"}, Artifacts: []model.ArtifactDecl{{Path: "out", Image: &model.ArtifactImage{Ref: "r"}}},
		Cache: &model.JobCacheConfig{Keys: []string{"k"}, Paths: []string{"p"}}, Requirements: &model.Requirements{Tools: []string{"go"}}}
	return j
}

func TestPlanDoesNotAliasConfigOrSiblings(t *testing.T) {
	cfg := cfgOf(map[string]model.Job{"m": mutableMatrixJob()})
	cfg.Support = []string{"linux/amd64", "darwin/arm64"}
	p := mustBuild(t, Input{World: linuxWorld("go"), Config: cfg})
	if len(p.Jobs) != 2 {
		t.Fatalf("jobs = %d", len(p.Jobs))
	}
	a, b := &p.Jobs[0].Job, &p.Jobs[1].Job
	a.Env["A"] = "changed"
	a.Run[0] = "changed"
	*a.Required = false
	a.Retry.InfraMax = 99
	a.When.Branches[0] = "changed"
	a.Matrix.Platform.([]any)[0] = "changed"
	a.Container.Image = "changed"
	a.Artifacts[0].Image.Ref = "changed"
	a.Cache.Keys[0] = "changed"
	a.Requirements.Tools[0] = "changed"
	a.NeedsServices[0] = "changed"
	if !reflect.DeepEqual(cfg.Jobs["m"], mutableMatrixJob()) {
		t.Error("mutating a planned job changed the caller's config")
	}
	var out []string
	shared(reflect.ValueOf(p.Jobs[0].Job), reflect.ValueOf(*b), "Job", &out)
	if len(out) > 0 || b.Env["A"] != "1" || b.Run[0] != "make" || !*b.Required || b.Retry.InfraMax != 1 {
		t.Errorf("matrix siblings share state: %v", out)
	}
	// Mutating the caller's config after Build must not reach the plan.
	cfg.Jobs["m"].Env["A"] = "late"
	cfg.Jobs["m"].When.Paths[0] = "late"
	if p.Jobs[1].Job.Env["A"] != "1" || p.Jobs[1].Job.When.Paths[0] != "a/**" {
		t.Error("the plan aliases the caller's config")
	}
	// The hook gets its own copy of the jobs.
	m := p.ModelPipeline()
	m.Jobs[1].Env["A"] = "hook"
	if p.Jobs[1].Job.Env["A"] != "1" {
		t.Error("ModelPipeline shares state with the plan")
	}
}
