package codegen

import (
	"strings"
	"testing"

	"github.com/meoyawn/chty-go/internal/parser"
	"github.com/meoyawn/chty-go/internal/schema"
)

func TestGenerateGoldenOutput(t *testing.T) {
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
		"func (r GetUserRow) GetUserID() int64",
		"clickhouse.Named(\"user_id\", userID)",
		"// chty:query\tGetUser\tone\t",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("generated output missing %q:\n%s", want, got)
		}
	}
}

func TestGenerateParamFormatting(t *testing.T) {
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
		"func (p ThreeParamsParams) args() []any",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("generated output missing %q:\n%s", want, got)
		}
	}
}
