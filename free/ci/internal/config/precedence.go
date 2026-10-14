package config

// Purpose: resolve the effective settings of a run with the precedence
// flag > env > file > detected default, recording the source of every value
// (`nself ci config show --json`, Epic D7).
// Inputs:  the flags the user set, an env lookup, the loaded file (or nil)
// and the detected defaults.
// Outputs: Effective, with a Source and Origin beside each value; or an
// *Error (E620) naming the flag or variable whose value is unusable.
// Constraints: pure (no environment or file access); every env binding is
// declared once in Bindings; the first non-empty layer wins, so an env value
// overrides a job's timeout in the file, as the precedence rule says.

import (
	"fmt"
	"strconv"
	"time"
)

// Source names the layer an effective value came from.
type Source string

// The layers, strongest first.
const (
	SourceFlag    Source = "flag"
	SourceEnv     Source = "env"
	SourceFile    Source = "file"
	SourceDefault Source = "default"
)

// DefaultTimeout is the job timeout when nothing else sets one.
const DefaultTimeout = "10m"

// Binding ties one setting to its flag and environment variable.
type Binding struct{ Name, Flag, Env string }

// Bindings declares every env variable of the config surface once.
// NSELF_CI_SHA belongs to v1.4 only and is deliberately absent.
var Bindings = []Binding{
	{"repo", "repo", "NSELF_CI_REPO"},
	{"timeout", "timeout", "NSELF_CI_TIMEOUT"},
	{"skip_status", "no-status", "NSELF_CI_SKIP_STATUS"},
	{"home", "home", "NSELF_CI_HOME"},
}

// Value is one effective setting and where it came from.
type Value struct {
	Value  any    `json:"value"`
	Source Source `json:"source"`
	Origin string `json:"origin,omitempty"`
}

// JobValues are the effective per-job settings.
type JobValues struct {
	Timeout       Value `json:"timeout"`
	Required      Value `json:"required"`
	Path          Value `json:"path"`
	RetryInfraMax Value `json:"retry_infra_max"`
	Priority      Value `json:"priority"`
	Idempotent    Value `json:"idempotent"`
	AllowFailure  Value `json:"allow_failure"`
	Workdir       Value `json:"workdir"`
	Isolation     Value `json:"isolation"`
}

// Effective is the resolved configuration.
type Effective struct {
	Repo       Value                `json:"repo"`
	Timeout    Value                `json:"timeout"`
	SkipStatus Value                `json:"skip_status"`
	Home       Value                `json:"home"`
	Jobs       map[string]JobValues `json:"jobs"`
}

// Defaults are the detected fallbacks.
type Defaults struct{ Repo, Home string }

// Inputs gathers the layers for Resolve. Flags holds only flags the user set.
type Inputs struct {
	Flags    map[string]string
	Env      func(string) string
	Loaded   *Loaded
	Defaults Defaults
}

// Resolve applies the precedence to every setting.
func Resolve(in Inputs) (Effective, error) {
	env := in.Env
	if env == nil {
		env = func(string) string { return "" }
	}
	bind := map[string]Binding{}
	for _, b := range Bindings {
		bind[b.Name] = b
	}
	// layer returns the strongest flag or env value for a binding.
	layer := func(name string) (string, Source, string) {
		b := bind[name]
		if v := in.Flags[b.Flag]; v != "" {
			return v, SourceFlag, "--" + b.Flag
		}
		if v := env(b.Env); v != "" {
			return v, SourceEnv, b.Env
		}
		return "", "", ""
	}
	bad := func(origin, msg string) (Effective, error) {
		return Effective{}, &Error{Code: CodeInvalid, Problems: []Problem{{File: "config", Pointer: origin, Message: msg}}}
	}
	var out Effective
	out.Repo = Value{in.Defaults.Repo, SourceDefault, "detected"}
	if v, src, origin := layer("repo"); src != "" {
		out.Repo = Value{v, src, origin}
	}
	out.Home = Value{in.Defaults.Home, SourceDefault, "detected"}
	if v, src, origin := layer("home"); src != "" {
		out.Home = Value{v, src, origin}
	}
	out.SkipStatus = Value{false, SourceDefault, "built-in"}
	if v, src, origin := layer("skip_status"); src != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return bad(origin, fmt.Sprintf("%q is not a boolean", v))
		}
		out.SkipStatus = Value{b, src, origin}
	}
	out.Timeout = Value{DefaultTimeout, SourceDefault, "built-in"}
	timeoutOverride, tsrc, torigin := layer("timeout")
	if tsrc != "" {
		if d, err := time.ParseDuration(timeoutOverride); err != nil || d <= 0 {
			return bad(torigin, fmt.Sprintf("%q is not a positive duration", timeoutOverride))
		}
		out.Timeout = Value{timeoutOverride, tsrc, torigin}
	}
	out.Jobs = map[string]JobValues{}
	if in.Loaded == nil {
		return out, nil
	}
	for _, id := range sortedKeys(in.Loaded.Config.Jobs) {
		job := in.Loaded.Config.Jobs[id]
		at := func(field string) string {
			return fmt.Sprintf("%s:%d", in.Loaded.Path, lineFor(in.Loaded.Lines, jobPtr(id, field)))
		}
		jv := JobValues{
			Timeout:       Value{DefaultTimeout, SourceDefault, "built-in"},
			Required:      Value{true, SourceDefault, "built-in"},
			Path:          Value{"deep", SourceDefault, "built-in"},
			RetryInfraMax: Value{1, SourceDefault, "built-in"},
			Priority:      Value{"normal", SourceDefault, "built-in"},
			Idempotent:    Value{true, SourceDefault, "built-in"},
			AllowFailure:  Value{false, SourceDefault, "built-in"},
			Workdir:       Value{".", SourceDefault, "built-in"},
			Isolation:     Value{"process", SourceDefault, "built-in"},
		}
		if job.Timeout != "" {
			jv.Timeout = Value{job.Timeout, SourceFile, at("/timeout")}
		}
		if tsrc != "" {
			jv.Timeout = out.Timeout
		}
		if job.Required != nil {
			jv.Required = Value{*job.Required, SourceFile, at("/required")}
		}
		if job.Path != "" {
			jv.Path = Value{string(job.Path), SourceFile, at("/path")}
		}
		if _, set := in.Loaded.Lines[jobPtr(id, "/retry/infra_max")]; set && job.Retry != nil {
			jv.RetryInfraMax = Value{job.Retry.InfraMax, SourceFile, at("/retry/infra_max")}
		}
		if job.Priority != "" {
			jv.Priority = Value{string(job.Priority), SourceFile, at("/priority")}
		}
		if job.Idempotent != nil {
			jv.Idempotent = Value{*job.Idempotent, SourceFile, at("/idempotent")}
		}
		if _, set := in.Loaded.Lines[jobPtr(id, "/allow_failure")]; set {
			jv.AllowFailure = Value{job.AllowFailure, SourceFile, at("/allow_failure")}
		}
		if job.Workdir != "" {
			jv.Workdir = Value{job.Workdir, SourceFile, at("/workdir")}
		}
		if job.Isolation != "" {
			jv.Isolation = Value{string(job.Isolation), SourceFile, at("/isolation")}
		}
		out.Jobs[id] = jv
	}
	return out, nil
}
