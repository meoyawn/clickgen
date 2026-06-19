package chtype

import "testing"

func TestMapPreservesIntegerWidths(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"Int8":    "int8",
		"Int16":   "int16",
		"Int32":   "int32",
		"Int64":   "int64",
		"UInt8":   "uint8",
		"UInt16":  "uint16",
		"UInt32":  "uint32",
		"UInt64":  "uint64",
		"Int128":  "big.Int",
		"UInt256": "big.Int",
	}
	for chType, want := range cases {
		if got := Map(chType, nil).Name; got != want {
			t.Fatalf("Map(%q) = %q, want %q", chType, got, want)
		}
	}
}

func TestMapComplexTypes(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"Date":                                 "time.Time",
		"DateTime64(3)":                        "time.Time",
		"Time64(6)":                            "time.Duration",
		"Nullable(Int32)":                      "*int32",
		"Array(Nullable(String))":              "[]*string",
		"Map(String, Array(UInt64))":           "map[string][]uint64",
		"LowCardinality(String)":               "string",
		"UUID":                                 "uuid.UUID",
		"Decimal(18, 4)":                       "decimal.Decimal",
		"Tuple(Int32, String)":                 "any",
		"Object('json')":                       "any",
		"Variant(Int32, String)":               "any",
		"SimpleAggregateFunction(sum, UInt64)": "uint64",
	}
	for chType, want := range cases {
		if got := Map(chType, nil).Name; got != want {
			t.Fatalf("Map(%q) = %q, want %q", chType, got, want)
		}
	}
}

func TestMapOverrides(t *testing.T) {
	t.Parallel()
	overrides := Overrides{}
	overrides.Add("JSON", "map[string]any")
	if got := Map("JSON", overrides).Name; got != "map[string]any" {
		t.Fatalf("override Map(JSON) = %q", got)
	}
}

func TestIsNullable(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"String":                            false,
		"Nullable(String)":                  true,
		"LowCardinality(String)":            false,
		"LowCardinality(Nullable(String))":  true,
		"Array(Nullable(String))":           false,
		"Nullable(Array(Nullable(String)))": true,
	}
	for chType, want := range cases {
		if got := IsNullable(chType); got != want {
			t.Fatalf("IsNullable(%q) = %t, want %t", chType, got, want)
		}
	}
}
