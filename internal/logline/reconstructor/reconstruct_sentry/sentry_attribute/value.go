// Adapted from https://github.com/open-telemetry/opentelemetry-go/blob/cc43e01c27892252aac9a8f20da28cdde957a289/attribute/value.go
//
// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sentry_attribute

import (
	"fmt"
	"strconv"

	"github.com/tidwall/sjson"
)

// Type describes the type of the data Value holds.
type Type int // redefines builtin Type.

// Value represents the value part in key-value pairs.
type Value struct {
	vtype    Type
	numeric  uint64
	stringly string
}

const (
	// INVALID is used for a Value with no value set.
	INVALID Type = iota
	// BOOL is a boolean Type Value.
	BOOL
	// INT64 is a 64-bit signed integral Type Value.
	INT64
	// FLOAT64 is a 64-bit floating point Type Value.
	FLOAT64
	// STRING is a string Type Value.
	STRING
)

// BoolValue creates a BOOL Value.
func BoolValue(v bool) Value {
	return Value{
		vtype:   BOOL,
		numeric: boolToRaw(v),
	}
}

// Int64Value creates an INT64 Value.
func Int64Value(v int64) Value {
	return Value{
		vtype:   INT64,
		numeric: int64ToRaw(v),
	}
}

// Float64Value creates a FLOAT64 Value.
func Float64Value(v float64) Value {
	return Value{
		vtype:   FLOAT64,
		numeric: float64ToRaw(v),
	}
}

// StringValue creates a STRING Value.
func StringValue(v string) Value {
	return Value{
		vtype:    STRING,
		stringly: v,
	}
}

// Type returns a type of the Value.
func (v Value) Type() Type {
	return v.vtype
}

// AsBool returns the bool value. Make sure that the Value's type is
// BOOL.
func (v Value) AsBool() bool {
	return rawToBool(v.numeric)
}

// AsInt64 returns the int64 value. Make sure that the Value's type is
// INT64.
func (v Value) AsInt64() int64 {
	return rawToInt64(v.numeric)
}

// AsFloat64 returns the float64 value. Make sure that the Value's
// type is FLOAT64.
func (v Value) AsFloat64() float64 {
	return rawToFloat64(v.numeric)
}

// AsString returns the string value. Make sure that the Value's type
// is STRING.
func (v Value) AsString() string {
	return v.stringly
}

const baseValueJSON = `{"value":"","type":""}`

type unknownValueType struct{}

// AsInterface returns Value's data as interface{}.
func (v Value) AsInterface() interface{} {
	switch v.Type() {
	case BOOL:
		return v.AsBool()
	case INT64:
		return v.AsInt64()
	case FLOAT64:
		return v.AsFloat64()
	case STRING:
		return v.stringly
	}
	return unknownValueType{}
}

// String returns a string representation of Value's data.
func (v Value) String() string {
	switch v.Type() {
	case BOOL:
		return strconv.FormatBool(v.AsBool())
	case INT64:
		return strconv.FormatInt(v.AsInt64(), 10)
	case FLOAT64:
		return fmt.Sprint(v.AsFloat64())
	case STRING:
		return v.stringly
	default:
		return "unknown"
	}
}

// RawJSON returns the Sentry attribute JSON encoding of the Value
// as raw bytes, built from the base template using sjson.
func (v Value) RawJSON() []byte {
	buf := []byte(baseValueJSON)
	switch v.vtype {
	case BOOL:
		buf, _ = sjson.SetBytes(buf, "value", v.AsBool())
		buf, _ = sjson.SetBytes(buf, "type", "boolean")
	case INT64:
		buf, _ = sjson.SetBytes(buf, "value", v.AsInt64())
		buf, _ = sjson.SetBytes(buf, "type", "integer")
	case FLOAT64:
		buf, _ = sjson.SetBytes(buf, "value", v.AsFloat64())
		buf, _ = sjson.SetBytes(buf, "type", "double")
	case STRING:
		buf, _ = sjson.SetBytes(buf, "value", v.stringly)
		buf, _ = sjson.SetBytes(buf, "type", "string")
	}
	return buf
}

// MarshalJSON returns the JSON encoding of the Value.
func (v Value) MarshalJSON() ([]byte, error) {
	return v.RawJSON(), nil
}

func (t Type) String() string {
	switch t {
	case BOOL:
		return "boolean"
	case INT64:
		return "integer"
	case FLOAT64:
		return "double"
	case STRING:
		return "string"
	}

	return "invalid"
}
