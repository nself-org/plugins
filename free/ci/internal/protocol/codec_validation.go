package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"unicode"
)

func strict(raw []byte, dst any) error {
	if err := uniqueKeys(raw); err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return err
	}
	if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
		return fmt.Errorf("trailing JSON")
	}
	return nil
}

// uniqueKeys walks tokens so duplicate and case-variant keys fail at every depth.
func uniqueKeys(raw []byte) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	if err := scanValue(d); err != nil {
		return err
	}
	if _, err := d.Token(); !errors.Is(err, io.EOF) {
		return invalid("trailing_json")
	}
	return nil
}

func scanValue(d *json.Decoder) error {
	t, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := t.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			s := foldKey(key.(string))
			if seen[s] {
				return invalid("duplicate_key")
			}
			seen[s] = true
			if err := scanValue(d); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err := scanValue(d); err != nil {
				return err
			}
		}
	default:
		return invalid("json_delimiter")
	}
	_, err = d.Token()
	return err
}

func foldKey(key string) string {
	var out strings.Builder
	out.Grow(len(key))
	for _, r := range key {
		min := r
		for next := unicode.SimpleFold(r); next != r; next = unicode.SimpleFold(next) {
			if next < min {
				min = next
			}
		}
		out.WriteRune(min)
	}
	return out.String()
}

func requiredFields(raw []byte, typ reflect.Type) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return invalid("null_body")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return invalid("body_object")
	}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		tag := strings.Split(field.Tag.Get("json"), ",")
		if len(tag) == 0 || tag[0] == "-" || tag[0] == "" {
			continue
		}
		if len(tag) > 1 && tag[1] == "omitempty" {
			continue
		}
		value, ok := fields[tag[0]]
		if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) && field.Type.Kind() != reflect.Pointer {
			return invalid("missing_field")
		}
		ft := field.Type
		if ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Struct && ft.PkgPath() == reflect.TypeFor[Hello]().PkgPath() {
			if err := requiredFields(value, ft); err != nil {
				return err
			}
		}
		if ft.Kind() == reflect.Slice && ft.Elem().Kind() == reflect.Struct && ft.Elem().PkgPath() == reflect.TypeFor[Hello]().PkgPath() {
			var items []json.RawMessage
			if err := json.Unmarshal(value, &items); err != nil {
				return err
			}
			for _, item := range items {
				if err := requiredFields(item, ft.Elem()); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validate(kind string, body any, epoch *uint64, leaseID string) error {
	switch v := body.(type) {
	case *Chunk:
		if v.Stream != "checkout" && v.Stream != "artifact" && v.Stream != "cache" {
			return invalid("chunk_stream")
		}
		if (v.Stream == "artifact" || v.Stream == "cache") && v.Digest == "" {
			return invalid("chunk_digest")
		}
	case *Digest:
		if !validStream(v.Stream) || v.SHA256 == "" || v.Size < 0 {
			return invalid("digest")
		}
	case *Offset:
		if !validStream(v.Stream) || v.SHA256 == "" || v.Offset < 0 {
			return invalid("offset")
		}
	case *Lease:
		if v.Epoch == 0 || v.LeaseID == "" || (v.CompatMode != Compat14 && v.CompatMode != Compat15) {
			return invalid("lease")
		}
		if epoch != nil && *epoch != v.Epoch || leaseID != "" && leaseID != v.LeaseID {
			return invalid("fencing_fields")
		}
		if v.Cache != nil && v.Cache.Access != "rw" && v.Cache.Access != "ro" && v.Cache.Access != "none" {
			return invalid("cache_access")
		}
		if v.RunnerCredential != nil && v.RunnerCredential.Kind != "github-jitconfig" && v.RunnerCredential.Kind != "gitlab-runner-token" {
			return invalid("runner_credential")
		}
	case *Result:
		if v.Epoch == 0 || v.LeaseID == "" {
			return invalid("result_epoch")
		}
		if epoch != nil && *epoch != v.Epoch || leaseID != "" && leaseID != v.LeaseID {
			return invalid("fencing_fields")
		}
	case *CacheRef:
		if v.LeaseID == "" || v.Spec == "" || v.Key == "" {
			return invalid("cache_ref")
		}
	}
	var bodyLease string
	var bodyEpoch uint64
	switch v := body.(type) {
	case *Lease:
		bodyLease, bodyEpoch = v.LeaseID, v.Epoch
	case *Result:
		bodyLease, bodyEpoch = v.LeaseID, v.Epoch
	case *Log:
		bodyLease = v.LeaseID
	case *ArtifactRef:
		bodyLease = v.LeaseID
	case *CacheRef:
		bodyLease = v.LeaseID
	case *Cancel:
		bodyLease = v.LeaseID
	}
	if bodyLease != "" || kind == "log" || kind == "artifact-ref" || kind == "cache-ref" || kind == "cancel" {
		if bodyLease == "" || leaseID != bodyLease || epoch == nil || *epoch == 0 || bodyEpoch != 0 && *epoch != bodyEpoch {
			return invalid("fencing_fields")
		}
	}
	if epoch != nil && *epoch == 0 {
		return invalid("epoch")
	}
	if kind == "lease" || kind == "result" {
		if epoch == nil || leaseID == "" {
			return invalid("fencing_fields")
		}
	}
	return nil
}
func validStream(s string) bool { return s == "checkout" || s == "artifact" || s == "cache" }
func validSecretRef(ref string) bool {
	return strings.HasPrefix(ref, "project/") && len(ref) > len("project/") || strings.HasPrefix(ref, "environment/") && len(ref) > len("environment/")
}

// Encoder writes canonical JSON keys using encoding/json's deterministic map order.
