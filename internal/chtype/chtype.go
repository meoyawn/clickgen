package chtype

import (
	"fmt"
	"strings"
	"unicode"
)

type GoType struct {
	Name    string
	Imports map[string]struct{}
}

type Overrides map[string]string

func ParseOverride(value string) (string, string, error) {
	left, right, ok := strings.Cut(value, "=")
	if !ok {
		return "", "", fmt.Errorf("expected ClickHouseType=GoType, got %q", value)
	}
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" {
		return "", "", fmt.Errorf("expected ClickHouseType=GoType, got %q", value)
	}
	return left, right, nil
}

func (o Overrides) Add(chType, goType string) {
	if o == nil {
		return
	}
	o[normalizeKey(chType)] = strings.TrimSpace(goType)
}

func Map(chType string, overrides Overrides) GoType {
	chType = strings.TrimSpace(chType)
	if overrides != nil {
		if goType, ok := overrides[normalizeKey(chType)]; ok {
			return custom(goType)
		}
	}
	return mapType(chType, overrides)
}

func DefaultLiteral(chType string) string {
	chType = strings.TrimSpace(chType)
	if fn, args, ok := function(chType); ok {
		switch fn {
		case "Nullable", "LowCardinality":
			if len(args) == 1 {
				return DefaultLiteral(args[0])
			}
		case "Array":
			return "[]"
		case "Map":
			return "map()"
		case "Date", "Date32":
			return "'1970-01-01'"
		case "DateTime", "DateTime64":
			return "'1970-01-01 00:00:00'"
		case "Time", "Time64":
			return "'00:00:00'"
		case "Decimal", "Decimal32", "Decimal64", "Decimal128", "Decimal256":
			return "0.0"
		case "FixedString", "String":
			return "''"
		}
	}

	switch {
	case strings.HasPrefix(chType, "Int"), strings.HasPrefix(chType, "UInt"):
		return "0"
	case strings.HasPrefix(chType, "Float"), strings.HasPrefix(chType, "Decimal"):
		return "0.0"
	case strings.HasPrefix(chType, "String"), strings.HasPrefix(chType, "FixedString"):
		return "''"
	case strings.HasPrefix(chType, "Bool"):
		return "false"
	case strings.HasPrefix(chType, "DateTime"):
		return "'1970-01-01 00:00:00'"
	case strings.HasPrefix(chType, "Date"):
		return "'1970-01-01'"
	case strings.HasPrefix(chType, "Time"):
		return "'00:00:00'"
	case strings.HasPrefix(chType, "UUID"):
		return "'00000000-0000-0000-0000-000000000000'"
	case strings.HasPrefix(chType, "Array"):
		return "[]"
	case strings.HasPrefix(chType, "Map"):
		return "map()"
	default:
		return "NULL"
	}
}

func mapType(chType string, overrides Overrides) GoType {
	if fn, args, ok := function(chType); ok {
		switch fn {
		case "Nullable":
			if len(args) == 1 {
				inner := Map(args[0], overrides)
				return GoType{Name: "*" + inner.Name, Imports: inner.Imports}
			}
		case "LowCardinality":
			if len(args) == 1 {
				return Map(args[0], overrides)
			}
		case "Array":
			if len(args) == 1 {
				inner := Map(args[0], overrides)
				return GoType{Name: "[]" + inner.Name, Imports: inner.Imports}
			}
		case "Map":
			if len(args) == 2 {
				key := Map(args[0], overrides)
				value := Map(args[1], overrides)
				return GoType{
					Name:    "map[" + key.Name + "]" + value.Name,
					Imports: mergeImports(key.Imports, value.Imports),
				}
			}
		case "SimpleAggregateFunction":
			if len(args) == 2 {
				return Map(args[1], overrides)
			}
		case "FixedString":
			return builtin("string")
		case "Date", "Date32", "DateTime", "DateTime64":
			return imported("time.Time", "time")
		case "Time", "Time64":
			return imported("time.Duration", "time")
		case "Decimal", "Decimal32", "Decimal64", "Decimal128", "Decimal256":
			return imported("decimal.Decimal", "github.com/shopspring/decimal")
		case "Enum", "Enum8", "Enum16":
			return builtin("string")
		case "Tuple", "Nested", "Variant", "Object", "AggregateFunction":
			return builtin("any")
		}
	}

	switch chType {
	case "Int8":
		return builtin("int8")
	case "Int16":
		return builtin("int16")
	case "Int32":
		return builtin("int32")
	case "Int64":
		return builtin("int64")
	case "Int128", "Int256":
		return imported("big.Int", "math/big")
	case "UInt8":
		return builtin("uint8")
	case "UInt16":
		return builtin("uint16")
	case "UInt32":
		return builtin("uint32")
	case "UInt64":
		return builtin("uint64")
	case "UInt128", "UInt256":
		return imported("big.Int", "math/big")
	case "Float32":
		return builtin("float32")
	case "Float64":
		return builtin("float64")
	case "String":
		return builtin("string")
	case "Bool":
		return builtin("bool")
	case "Date", "Date32", "DateTime", "DateTime64":
		return imported("time.Time", "time")
	case "Time", "Time64":
		return imported("time.Duration", "time")
	case "UUID":
		return imported("uuid.UUID", "github.com/google/uuid")
	case "IPv4", "IPv6", "Enum", "Enum8", "Enum16":
		return builtin("string")
	case "JSON", "Dynamic":
		return builtin("any")
	}

	if strings.HasPrefix(chType, "Decimal") {
		return imported("decimal.Decimal", "github.com/shopspring/decimal")
	}
	if strings.HasPrefix(chType, "FixedString") {
		return builtin("string")
	}
	if strings.HasPrefix(chType, "Enum") {
		return builtin("string")
	}
	if strings.HasPrefix(chType, "Object") ||
		strings.HasPrefix(chType, "Tuple") ||
		strings.HasPrefix(chType, "Nested") ||
		strings.HasPrefix(chType, "Variant") ||
		strings.HasPrefix(chType, "AggregateFunction") {
		return builtin("any")
	}

	return builtin("any")
}

func builtin(name string) GoType {
	return GoType{Name: name, Imports: map[string]struct{}{}}
}

func imported(name, importPath string) GoType {
	return GoType{Name: name, Imports: map[string]struct{}{importPath: {}}}
}

func custom(name string) GoType {
	imports := map[string]struct{}{}
	switch {
	case strings.Contains(name, "time."):
		imports["time"] = struct{}{}
	case strings.Contains(name, "big."):
		imports["math/big"] = struct{}{}
	case strings.Contains(name, "uuid."):
		imports["github.com/google/uuid"] = struct{}{}
	case strings.Contains(name, "decimal."):
		imports["github.com/shopspring/decimal"] = struct{}{}
	}
	return GoType{Name: name, Imports: imports}
}

func mergeImports(sets ...map[string]struct{}) map[string]struct{} {
	out := map[string]struct{}{}
	for _, set := range sets {
		for path := range set {
			out[path] = struct{}{}
		}
	}
	return out
}

func function(chType string) (string, []string, bool) {
	open := strings.IndexByte(chType, '(')
	if open < 0 || !strings.HasSuffix(chType, ")") {
		return chType, nil, true
	}

	name := strings.TrimSpace(chType[:open])
	inner := chType[open+1 : len(chType)-1]
	if name == "" || !balanced(inner) {
		return "", nil, false
	}
	return name, splitTopLevel(inner), true
}

func splitTopLevel(input string) []string {
	if strings.TrimSpace(input) == "" {
		return nil
	}
	var parts []string
	start := 0
	depth := 0
	quote := rune(0)
	for idx, r := range input {
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(input[start:idx]))
				start = idx + len(string(r))
			}
		}
	}
	parts = append(parts, strings.TrimSpace(input[start:]))
	return parts
}

func balanced(input string) bool {
	depth := 0
	quote := rune(0)
	for _, r := range input {
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
		case '(':
			depth++
		case ')':
			depth--
			if depth < 0 {
				return false
			}
		}
	}
	return depth == 0 && quote == 0
}

func normalizeKey(input string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(input) {
		if !unicode.IsSpace(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
