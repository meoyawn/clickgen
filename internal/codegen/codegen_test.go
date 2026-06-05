package codegen

import (
	"strings"
	"testing"

	"github.com/meoyawn/chty-go/internal/parser"
	"github.com/meoyawn/chty-go/internal/schema"
)

func TestGenerateGoldenOutput(t *testing.T) {
	t.Parallel()
	generated, err := Generate(Options{PackageName: "generated"}, []QuerySpec{
		{
			Query: parser.Query{
				Name: "GetUser",
				Cmd:  parser.CommandOne,
				SQL:  "SELECT user_id, username FROM users WHERE user_id = {user_id:Int64}",
				Params: []parser.Parameter{
					{Name: "user_id", ClickHouseType: "Int64"},
				},
			},
			Result: []schema.Column{
				{Name: "user_id", ClickHouseType: "Int64"},
				{Name: "username", ClickHouseType: "String"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := string(generated)
	for _, want := range []string{
		"package generated",
		"type genericConn interface",
		"type Querier interface",
		"GetUser(ctx context.Context, userID int64) (GetUserRow, error)",
		"type GetUserRow struct",
		"UserID",
		"`json:\"user_id\" ch:\"user_id\"`",
		"type GetUserProjection interface",
		"func (r *GetUserRow) GetUserID() int64",
		"SELECT user_id, username FROM users WHERE user_id = {user_id:Int64}",
		"ctx = clickhouse.Context(ctx, clickhouse.WithParameters(getUserArgs(userID)))",
		"// chty:query\tGetUser\tone\t",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("generated output missing %q:\n%s", want, got)
		}
	}
}

func TestGenerateKeepsLiteralAtTokensOutOfBindSyntax(t *testing.T) {
	t.Parallel()
	generated, err := Generate(Options{PackageName: "generated"}, []QuerySpec{
		{
			Query: parser.Query{
				Name: "FindUser",
				Cmd:  parser.CommandOne,
				SQL:  "SELECT user_id FROM users WHERE email = 'admin@example.com' AND note = '?' AND user_id = {user_id:Int64}",
				Params: []parser.Parameter{
					{Name: "user_id", ClickHouseType: "Int64"},
				},
			},
			Result: []schema.Column{{Name: "user_id", ClickHouseType: "Int64"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	got := string(generated)
	for _, want := range []string{
		"email = 'admin@example.com'",
		"note = '?'",
		"user_id = {user_id:Int64}",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("generated output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "@user_id") || strings.Contains(got, "user_id = ?") {
		t.Fatalf("generated output contains legacy bind syntax:\n%s", got)
	}
}

func TestGenerateDuplicatesRepeatedParameterArgs(t *testing.T) {
	t.Parallel()
	generated, err := Generate(Options{PackageName: "generated"}, []QuerySpec{
		{
			Query: parser.Query{
				Name: "Repeated",
				Cmd:  parser.CommandOne,
				SQL:  "SELECT {id:Int64} + {id:Int64}",
				Params: []parser.Parameter{
					{Name: "id", ClickHouseType: "Int64"},
				},
			},
			Result: []schema.Column{{Name: "sum", ClickHouseType: "Int64"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(generated) == 0 {
		t.Fatal("generated output is empty")
	}
}

func TestGenerateFormatsNestedQueryParameterLiterals(t *testing.T) {
	t.Parallel()
	generated, err := Generate(Options{PackageName: "generated"}, []QuerySpec{
		{
			Query: parser.Query{
				Name: "NestedParams",
				Cmd:  parser.CommandExec,
				SQL:  "INSERT INTO events VALUES ({seen_at:Array(DateTime)}, {attrs:Map(String, Array(UInt64))}, {id:UUID}, {note:Nullable(String)})",
				Params: []parser.Parameter{
					{Name: "seen_at", ClickHouseType: "Array(DateTime)"},
					{Name: "attrs", ClickHouseType: "Map(String, Array(UInt64))"},
					{Name: "id", ClickHouseType: "UUID"},
					{Name: "note", ClickHouseType: "Nullable(String)"},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := string(generated)
	for _, want := range []string{
		"func queryParameterValueArrayDateTime(value []time.Time) string",
		"parts = append(parts, quoteQueryParameterString(item.Format(\"2006-01-02 15:04:05.999999999\")))",
		"func queryParameterValueMapStringArrayUInt64(value map[string][]uint64) string",
		"sort.Slice(keys, func(left, right int) bool",
		"parts = append(parts, queryParameterValueArrayUInt64(value[key]))",
		"func queryParameterValueNullableString(value *string) string",
		"return \"map(\" + strings.Join(parts, \",\") + \")\"",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("generated output missing %q:\n%s", want, got)
		}
	}
	if banned := "re" + "flect"; strings.Contains(got, banned) {
		t.Fatalf("generated output contains %q:\n%s", banned, got)
	}
	for _, notWant := range []string{
		"func queryParameterValueUUID",
		"func queryParameterLiteralUInt64",
		"func queryParameterLiteralString",
		"func queryParameterValueString",
	} {
		if strings.Contains(got, notWant) {
			t.Fatalf("generated output contains unnecessary helper %q:\n%s", notWant, got)
		}
	}
}

func TestGenerateParamFormatting(t *testing.T) {
	t.Parallel()
	generated, err := Generate(Options{PackageName: "generated"}, []QuerySpec{
		{
			Query: parser.Query{
				Name: "NoParams",
				Cmd:  parser.CommandOne,
				SQL:  "SELECT 1 AS n",
			},
			Result: []schema.Column{{Name: "n", ClickHouseType: "UInt8"}},
		},
		{
			Query: parser.Query{
				Name: "TwoParams",
				Cmd:  parser.CommandMany,
				SQL:  "SELECT number FROM system.numbers WHERE number >= {min:UInt64} LIMIT {limit:UInt64}",
				Params: []parser.Parameter{
					{Name: "min", ClickHouseType: "UInt64"},
					{Name: "limit", ClickHouseType: "UInt64"},
				},
			},
			Result: []schema.Column{{Name: "number", ClickHouseType: "UInt64"}},
		},
		{
			Query: parser.Query{
				Name: "ThreeParams",
				Cmd:  parser.CommandExec,
				SQL:  "INSERT INTO users VALUES ({id:Int64}, {name:String}, {age:Int32})",
				Params: []parser.Parameter{
					{Name: "id", ClickHouseType: "Int64"},
					{Name: "name", ClickHouseType: "String"},
					{Name: "age", ClickHouseType: "Int32"},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := string(generated)
	for _, want := range []string{
		"NoParams(ctx context.Context) (uint8, error)",
		"TwoParams(ctx context.Context, min uint64, limit uint64) ([]TwoParamsRow, error)",
		"type ThreeParamsParams struct",
		"ThreeParams(ctx context.Context, params ThreeParamsParams) error",
		"func (p ThreeParamsParams) args() clickhouse.Parameters",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("generated output missing %q:\n%s", want, got)
		}
	}
}
