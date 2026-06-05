package codegen

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/meoyawn/chty-go/internal/chtype"
)

type queryParameterKind int

const (
	queryParameterOpaque queryParameterKind = iota
	queryParameterInt
	queryParameterUint
	queryParameterFloat
	queryParameterBool
	queryParameterString
	queryParameterTime
	queryParameterDuration
	queryParameterUUID
	queryParameterNullable
	queryParameterArray
	queryParameterMap
)

type queryParameterType struct {
	kind      queryParameterKind
	chType    string
	goType    string
	name      string
	args      []queryParameterType
	valueFn   string
	literalFn string
	bits      int
}

type queryParameterFormatterRegistry struct {
	overrides    chtype.Overrides
	valueSeen    map[string]struct{}
	valueOrder   []queryParameterType
	literalSeen  map[string]struct{}
	literalOrder []queryParameterType
	imports      map[string]struct{}
}

func collectQueryParameterFormatters(models []queryModel, overrides chtype.Overrides) queryParameterFormatterRegistry {
	registry := queryParameterFormatterRegistry{
		overrides:   overrides,
		valueSeen:   map[string]struct{}{},
		literalSeen: map[string]struct{}{},
		imports:     map[string]struct{}{},
	}
	for _, model := range models {
		for _, param := range model.Params {
			registry.valueExpr(param.CHType, param.LocalName)
		}
	}
	return registry
}

func (r *queryParameterFormatterRegistry) valueExpr(chType, expr string) string {
	t := r.parse(chType)
	if out, ok := r.inlineValueExpr(t, expr); ok {
		return out
	}
	r.requireValue(t)
	return t.valueFn + "(" + expr + ")"
}

func (r *queryParameterFormatterRegistry) literalExpr(t queryParameterType, expr string) string {
	if out, ok := r.inlineLiteralExpr(t, expr); ok {
		return out
	}
	if t.kind == queryParameterArray || t.kind == queryParameterMap {
		r.requireValue(t)
		return t.valueFn + "(" + expr + ")"
	}
	r.requireLiteral(t)
	return t.literalFn + "(" + expr + ")"
}

func (r *queryParameterFormatterRegistry) requireValue(t queryParameterType) {
	if out, ok := r.inlineValueExpr(t, "value"); ok {
		_ = out
		return
	}
	if _, ok := r.valueSeen[t.valueFn]; ok {
		return
	}
	r.valueSeen[t.valueFn] = struct{}{}
	switch t.kind {
	case queryParameterNullable:
		r.requireValueDependency(t.args[0])
	case queryParameterArray:
		r.imports["strings"] = struct{}{}
		r.requireLiteral(t.args[0])
	case queryParameterMap:
		r.imports["sort"] = struct{}{}
		r.imports["strings"] = struct{}{}
		r.requireLiteral(t.args[0])
		r.requireLiteral(t.args[1])
	case queryParameterOpaque, queryParameterInt, queryParameterUint, queryParameterFloat, queryParameterBool, queryParameterString, queryParameterTime, queryParameterDuration, queryParameterUUID:
	}
	r.valueOrder = append(r.valueOrder, t)
}

func (r *queryParameterFormatterRegistry) requireLiteral(t queryParameterType) {
	if out, ok := r.inlineLiteralExpr(t, "value"); ok {
		_ = out
		return
	}
	if t.kind == queryParameterArray || t.kind == queryParameterMap {
		r.requireValue(t)
		return
	}
	if _, ok := r.literalSeen[t.literalFn]; ok {
		return
	}
	r.literalSeen[t.literalFn] = struct{}{}
	if t.kind == queryParameterNullable {
		r.requireLiteral(t.args[0])
	}
	r.literalOrder = append(r.literalOrder, t)
}

func (r *queryParameterFormatterRegistry) requireValueDependency(t queryParameterType) {
	if out, ok := r.inlineValueExpr(t, "value"); ok {
		_ = out
		return
	}
	r.requireValue(t)
}

func (r *queryParameterFormatterRegistry) parse(chType string) queryParameterType {
	chType = strings.TrimSpace(chType)
	goType := chtype.Map(chType, r.overrides).Name
	suffix := queryParameterFormatterSuffix(chType)
	t := queryParameterType{
		kind:      queryParameterOpaque,
		chType:    chType,
		goType:    goType,
		name:      chType,
		valueFn:   "queryParameterValue" + suffix,
		literalFn: "queryParameterLiteral" + suffix,
	}
	if r.hasOverride(chType) {
		return t
	}

	name, args := queryParameterFunction(chType)
	if len(args) > 0 {
		t.name = name
		switch name {
		case "Nullable":
			if len(args) == 1 {
				t.kind = queryParameterNullable
				t.args = []queryParameterType{r.parse(args[0])}
			}
			return t
		case "LowCardinality":
			if len(args) == 1 {
				return r.parse(args[0])
			}
		case "Array":
			if len(args) == 1 {
				t.kind = queryParameterArray
				t.args = []queryParameterType{r.parse(args[0])}
			}
			return t
		case "Map":
			if len(args) == 2 {
				t.kind = queryParameterMap
				t.args = []queryParameterType{r.parse(args[0]), r.parse(args[1])}
			}
			return t
		case "SimpleAggregateFunction":
			if len(args) == 2 {
				return r.parse(args[1])
			}
		case "Date", "Date32", "DateTime", "DateTime64":
			t.kind = queryParameterTime
			return t
		case "Time", "Time64":
			t.kind = queryParameterDuration
			return t
		case "FixedString", "String", "Enum", "Enum8", "Enum16", "IPv4", "IPv6":
			t.kind = queryParameterString
			return t
		case "Decimal", "Decimal32", "Decimal64", "Decimal128", "Decimal256":
			return t
		}
	}

	switch chType {
	case "Int8":
		t.kind, t.bits = queryParameterInt, 8
	case "Int16":
		t.kind, t.bits = queryParameterInt, 16
	case "Int32":
		t.kind, t.bits = queryParameterInt, 32
	case "Int64":
		t.kind, t.bits = queryParameterInt, 64
	case "UInt8":
		t.kind, t.bits = queryParameterUint, 8
	case "UInt16":
		t.kind, t.bits = queryParameterUint, 16
	case "UInt32":
		t.kind, t.bits = queryParameterUint, 32
	case "UInt64":
		t.kind, t.bits = queryParameterUint, 64
	case "Float32":
		t.kind, t.bits = queryParameterFloat, 32
	case "Float64":
		t.kind, t.bits = queryParameterFloat, 64
	case "Bool":
		t.kind = queryParameterBool
	case "String", "IPv4", "IPv6":
		t.kind = queryParameterString
	case "Date", "Date32", "DateTime", "DateTime64":
		t.kind = queryParameterTime
	case "Time", "Time64":
		t.kind = queryParameterDuration
	case "UUID":
		t.kind = queryParameterUUID
	}
	return t
}

func (r *queryParameterFormatterRegistry) hasOverride(chType string) bool {
	if r.overrides == nil {
		return false
	}
	_, ok := r.overrides[queryParameterOverrideKey(chType)]
	return ok
}

func (r *queryParameterFormatterRegistry) inlineValueExpr(t queryParameterType, expr string) (string, bool) {
	switch t.kind {
	case queryParameterOpaque:
		r.imports["fmt"] = struct{}{}
		return "fmt.Sprint(" + expr + ")", true
	case queryParameterInt:
		r.imports["strconv"] = struct{}{}
		return "strconv.FormatInt(int64(" + expr + "), 10)", true
	case queryParameterUint:
		r.imports["strconv"] = struct{}{}
		return "strconv.FormatUint(uint64(" + expr + "), 10)", true
	case queryParameterFloat:
		r.imports["strconv"] = struct{}{}
		return fmt.Sprintf("strconv.FormatFloat(float64(%s), 'f', -1, %d)", expr, t.bits), true
	case queryParameterBool:
		r.imports["strconv"] = struct{}{}
		return "strconv.FormatBool(" + expr + ")", true
	case queryParameterString:
		return expr, true
	case queryParameterTime:
		return expr + `.Format("2006-01-02 15:04:05.999999999")`, true
	case queryParameterDuration:
		r.imports["fmt"] = struct{}{}
		r.imports["time"] = struct{}{}
		return "queryParameterDuration(" + expr + ")", true
	case queryParameterUUID:
		return expr + ".String()", true
	case queryParameterNullable, queryParameterArray, queryParameterMap:
	}
	return "", false
}

func (r *queryParameterFormatterRegistry) inlineLiteralExpr(t queryParameterType, expr string) (string, bool) {
	switch t.kind {
	case queryParameterString:
		r.imports["strings"] = struct{}{}
		return "quoteQueryParameterString(" + expr + ")", true
	case queryParameterTime, queryParameterDuration, queryParameterUUID:
		r.imports["strings"] = struct{}{}
		valueExpr, _ := r.inlineValueExpr(t, expr)
		return "quoteQueryParameterString(" + valueExpr + ")", true
	case queryParameterOpaque, queryParameterInt, queryParameterUint, queryParameterFloat, queryParameterBool:
		return r.inlineValueExpr(t, expr)
	case queryParameterNullable, queryParameterArray, queryParameterMap:
	}
	return "", false
}

func (r *queryParameterFormatterRegistry) write(b *strings.Builder) {
	for _, t := range r.valueOrder {
		r.writeValueType(b, t)
	}
	for _, t := range r.literalOrder {
		r.writeLiteralType(b, t)
	}
	if _, ok := r.imports["time"]; ok {
		writeQueryParameterDuration(b)
	}
	if _, ok := r.imports["strings"]; ok {
		writeQuoteQueryParameterString(b)
	}
}

func (r *queryParameterFormatterRegistry) writeValueType(b *strings.Builder, t queryParameterType) {
	switch t.kind {
	case queryParameterOpaque:
		writeQueryParameterValueFn(b, t, "return fmt.Sprint(value)")
	case queryParameterInt:
		writeQueryParameterValueFn(b, t, "return strconv.FormatInt(int64(value), 10)")
	case queryParameterUint:
		writeQueryParameterValueFn(b, t, "return strconv.FormatUint(uint64(value), 10)")
	case queryParameterFloat:
		writeQueryParameterValueFn(b, t, fmt.Sprintf("return strconv.FormatFloat(float64(value), 'f', -1, %d)", t.bits))
	case queryParameterBool:
		writeQueryParameterValueFn(b, t, "return strconv.FormatBool(value)")
	case queryParameterString:
		writeQueryParameterValueFn(b, t, "return value")
	case queryParameterTime:
		writeQueryParameterValueFn(b, t, `return value.Format("2006-01-02 15:04:05.999999999")`)
	case queryParameterDuration:
		writeQueryParameterValueFn(b, t, "return queryParameterDuration(value)")
	case queryParameterUUID:
		writeQueryParameterValueFn(b, t, "return value.String()")
	case queryParameterNullable:
		inner := t.args[0]
		writeQueryParameterValueFn(b, t, "if value == nil {\n\t\treturn \"NULL\"\n\t}\n\treturn "+r.valueExpr(inner.chType, "*value"))
	case queryParameterArray:
		inner := t.args[0]
		r.writeQueryParameterArrayFn(b, t, inner)
	case queryParameterMap:
		r.writeQueryParameterMapFn(b, t)
	}
}

func (r *queryParameterFormatterRegistry) writeLiteralType(b *strings.Builder, t queryParameterType) {
	switch t.kind {
	case queryParameterNullable:
		inner := t.args[0]
		writeQueryParameterLiteralFn(b, t, "if value == nil {\n\t\treturn \"NULL\"\n\t}\n\treturn "+r.literalExpr(inner, "*value"))
	case queryParameterOpaque, queryParameterInt, queryParameterUint, queryParameterFloat, queryParameterBool, queryParameterString, queryParameterTime, queryParameterDuration, queryParameterUUID, queryParameterArray, queryParameterMap:
		writeQueryParameterLiteralFn(b, t, "return "+r.literalExpr(t, "value"))
	}
}

func writeQueryParameterValueFn(b *strings.Builder, t queryParameterType, body string) {
	b.WriteString("func ")
	b.WriteString(t.valueFn)
	b.WriteString("(value ")
	b.WriteString(t.goType)
	b.WriteString(") string {\n\t")
	b.WriteString(body)
	b.WriteString("\n}\n\n")
}

func writeQueryParameterLiteralFn(b *strings.Builder, t queryParameterType, body string) {
	b.WriteString("func ")
	b.WriteString(t.literalFn)
	b.WriteString("(value ")
	b.WriteString(t.goType)
	b.WriteString(") string {\n\t")
	b.WriteString(body)
	b.WriteString("\n}\n\n")
}

func (r *queryParameterFormatterRegistry) writeQueryParameterArrayFn(b *strings.Builder, t, inner queryParameterType) {
	b.WriteString("func ")
	b.WriteString(t.valueFn)
	b.WriteString("(value ")
	b.WriteString(t.goType)
	b.WriteString(") string {\n")
	b.WriteString("\tparts := make([]string, 0, len(value))\n")
	b.WriteString("\tfor _, item := range value {\n")
	b.WriteString("\t\tparts = append(parts, ")
	b.WriteString(r.literalExpr(inner, "item"))
	b.WriteString(")\n")
	b.WriteString("\t}\n")
	b.WriteString("\treturn \"[\" + strings.Join(parts, \",\") + \"]\"\n")
	b.WriteString("}\n\n")
}

func (r *queryParameterFormatterRegistry) writeQueryParameterMapFn(b *strings.Builder, t queryParameterType) {
	key := t.args[0]
	value := t.args[1]
	b.WriteString("func ")
	b.WriteString(t.valueFn)
	b.WriteString("(value ")
	b.WriteString(t.goType)
	b.WriteString(") string {\n")
	b.WriteString("\tkeys := make([]")
	b.WriteString(key.goType)
	b.WriteString(", 0, len(value))\n")
	b.WriteString("\tfor key := range value {\n")
	b.WriteString("\t\tkeys = append(keys, key)\n")
	b.WriteString("\t}\n")
	b.WriteString("\tsort.Slice(keys, func(left, right int) bool {\n")
	b.WriteString("\t\treturn ")
	b.WriteString(r.literalExpr(key, "keys[left]"))
	b.WriteString(" < ")
	b.WriteString(r.literalExpr(key, "keys[right]"))
	b.WriteString("\n")
	b.WriteString("\t})\n\n")
	b.WriteString("\tparts := make([]string, 0, len(keys)*2)\n")
	b.WriteString("\tfor _, key := range keys {\n")
	b.WriteString("\t\tparts = append(parts, ")
	b.WriteString(r.literalExpr(key, "key"))
	b.WriteString(")\n")
	b.WriteString("\t\tparts = append(parts, ")
	b.WriteString(r.literalExpr(value, "value[key]"))
	b.WriteString(")\n")
	b.WriteString("\t}\n")
	b.WriteString("\treturn \"map(\" + strings.Join(parts, \",\") + \")\"\n")
	b.WriteString("}\n\n")
}

func writeQueryParameterDuration(b *strings.Builder) {
	b.WriteString(`func queryParameterDuration(value time.Duration) string {
	sign := ""
	if value < 0 {
		sign = "-"
		value = -value
	}
	totalSeconds := int64(value / time.Second)
	nanos := int64(value % time.Second)
	if nanos == 0 {
		return fmt.Sprintf("%s%02d:%02d:%02d", sign, totalSeconds/3600, totalSeconds/60%60, totalSeconds%60)
	}
	return fmt.Sprintf("%s%02d:%02d:%02d.%09d", sign, totalSeconds/3600, totalSeconds/60%60, totalSeconds%60, nanos)
}

`)
}

func writeQuoteQueryParameterString(b *strings.Builder) {
	b.WriteString(`func quoteQueryParameterString(value string) string {
	escaped := strings.ReplaceAll(value, ` + strconv.Quote(`\`) + `, ` + strconv.Quote(`\\`) + `)
	escaped = strings.ReplaceAll(escaped, "'", ` + strconv.Quote(`\'`) + `)
	return "'" + escaped + "'"
}

`)
}

func queryParameterFunction(chType string) (string, []string) {
	open := strings.IndexByte(chType, '(')
	if open < 0 || !strings.HasSuffix(chType, ")") {
		return chType, nil
	}

	name := strings.TrimSpace(chType[:open])
	inner := chType[open+1 : len(chType)-1]
	var args []string
	start := 0
	depth := 0
	for idx, r := range inner {
		switch r {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				args = append(args, strings.TrimSpace(inner[start:idx]))
				start = idx + 1
			}
		}
	}
	args = append(args, strings.TrimSpace(inner[start:]))
	return name, args
}

func queryParameterFormatterSuffix(chType string) string {
	name, args := queryParameterFunction(chType)
	if len(args) > 0 {
		var b strings.Builder
		b.WriteString(queryParameterIdentifierPart(name))
		for _, arg := range args {
			b.WriteString(queryParameterFormatterSuffix(arg))
		}
		return b.String()
	}
	return queryParameterIdentifierPart(chType)
}

func queryParameterIdentifierPart(value string) string {
	words := splitWords(value)
	if len(words) == 0 {
		return "Opaque"
	}
	var b strings.Builder
	for _, word := range words {
		lower := strings.ToLower(word)
		if initialism, ok := goInitialisms[lower]; ok {
			b.WriteString(initialism)
			continue
		}
		b.WriteString(strings.ToUpper(lower[:1]))
		if len(lower) > 1 {
			b.WriteString(lower[1:])
		}
	}
	return b.String()
}

func queryParameterOverrideKey(chType string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(chType) {
		if unicodeIsSpace(r) {
			continue
		}
		b.WriteRune(r)
	}
	return strings.ToLower(b.String())
}

func unicodeIsSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\v' || r == '\f'
}
