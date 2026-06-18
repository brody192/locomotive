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

func (v Value) asBool() bool       { return rawToBool(v.numeric) }
func (v Value) asInt64() int64     { return rawToInt64(v.numeric) }
func (v Value) asFloat64() float64 { return rawToFloat64(v.numeric) }

const baseValueJSON = `{"value":"","type":""}`

// RawJSON returns the Sentry attribute JSON encoding of the Value as raw bytes, built
// from the base template using sjson.
func (v Value) RawJSON() []byte {
	buf := []byte(baseValueJSON)
	switch v.vtype {
	case BOOL:
		buf, _ = sjson.SetBytes(buf, "value", v.asBool())
		buf, _ = sjson.SetBytes(buf, "type", "boolean")
	case INT64:
		buf, _ = sjson.SetBytes(buf, "value", v.asInt64())
		buf, _ = sjson.SetBytes(buf, "type", "integer")
	case FLOAT64:
		buf, _ = sjson.SetBytes(buf, "value", v.asFloat64())
		buf, _ = sjson.SetBytes(buf, "type", "double")
	case STRING:
		buf, _ = sjson.SetBytes(buf, "value", v.stringly)
		buf, _ = sjson.SetBytes(buf, "type", "string")
	}
	return buf
}
