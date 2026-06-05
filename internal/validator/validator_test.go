package validator

import (
	"strings"
	"testing"

	"github.com/meoyawn/clickgen/internal/parser"
	"github.com/meoyawn/clickgen/internal/schema"
)

func TestValidateParameterDiffs(t *testing.T) {
	t.Parallel()
	query := GeneratedQuery{
		Name: "Search",
		Kind: parser.CommandMany,
		SQL:  "SELECT number FROM system.numbers WHERE number >= {min:UInt64} AND number < {limit:UInt64}",
		Params: []Parameter{
			{Name: "min", ClickHouseType: "UInt64", GoType: "string"},
			{Name: "extra", ClickHouseType: "String", GoType: "string"},
		},
	}
	errors := validateParameters(query, nil)
	joined := strings.Join(errors, "\n")
	for _, want := range []string{
		"Query parameter missing from generated parameters: limit",
		"Generated parameters has parameter not in query: extra",
		"Parameter type mismatch for 'min': query expects uint64, generated parameters has string",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("errors missing %q: %#v", want, errors)
		}
	}
}

func TestValidateResultSchemaDiffs(t *testing.T) {
	t.Parallel()
	query := GeneratedQuery{
		Name: "Search",
		Kind: parser.CommandMany,
		Results: []ResultColumn{
			{Name: "number", ClickHouseType: "UInt64", GoType: "string"},
			{Name: "missing", ClickHouseType: "String", GoType: "string"},
		},
	}
	actual := []schema.Column{
		{Name: "number", ClickHouseType: "UInt64"},
		{Name: "extra", ClickHouseType: "String"},
	}
	errors := validateResultSchema(query, actual, nil)
	joined := strings.Join(errors, "\n")
	for _, want := range []string{
		"Generated result expects column not in query result: missing",
		"Query result has column not in generated result: extra",
		"Result type mismatch for 'number': query returns uint64, generated result expects string",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("errors missing %q: %#v", want, errors)
		}
	}
}
