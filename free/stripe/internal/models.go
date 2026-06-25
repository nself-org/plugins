package internal

import (
	"encoding/json"
	"time"
)

// NullTime represents a nullable timestamp for JSON serialization.
type NullTime struct {
	Time  time.Time
	Valid bool
}

func (nt NullTime) MarshalJSON() ([]byte, error) {
	if !nt.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(nt.Time)
}

// NullString represents a nullable string for JSON serialization.
type NullString struct {
	String string
	Valid  bool
}

func (ns NullString) MarshalJSON() ([]byte, error) {
	if !ns.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(ns.String)
}

// NullInt64 represents a nullable int64 for JSON serialization.
type NullInt64 struct {
	Int64 int64
	Valid bool
}

func (ni NullInt64) MarshalJSON() ([]byte, error) {
	if !ni.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(ni.Int64)
}

// NullBool represents a nullable bool for JSON serialization.
type NullBool struct {
	Bool  bool
	Valid bool
}

func (nb NullBool) MarshalJSON() ([]byte, error) {
	if !nb.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(nb.Bool)
}

// NullFloat64 represents a nullable float64 for JSON serialization.
type NullFloat64 struct {
	Float64 float64
	Valid   bool
}

func (nf NullFloat64) MarshalJSON() ([]byte, error) {
	if !nf.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(nf.Float64)
}

// NullInt32 represents a nullable int32 for JSON serialization.
type NullInt32 struct {
	Int32 int32
	Valid bool
}

func (ni NullInt32) MarshalJSON() ([]byte, error) {
	if !ni.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(ni.Int32)
}

// StripeCustomer maps to np_stripe_customers.
