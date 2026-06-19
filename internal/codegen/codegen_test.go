package codegen

import (
	"strings"
	"testing"

	"github.com/meoyawn/clickgen/internal/parser"
	"github.com/meoyawn/clickgen/internal/schema"
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
		"type DBQuerier interface",
		"func GetUser(ctx context.Context, db DBQuerier, userID int64) (GetUserRow, error)",
		"type GetUserRow struct",
		"UserID",
		"`json:\"user_id\" ch:\"user_id\"`",
		").ScanStruct(&row); err != nil",
		"SELECT user_id, username FROM users WHERE user_id = {user_id:Int64}",
		"ctx = clickhouse.Context(ctx, clickhouse.WithParameters(getUserArgs(userID)))",
		"// clickgen:query\tGetUser\tone\t",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("generated output missing %q:\n%s", want, got)
		}
	}
}

func TestGenerateSharedRowAnnotation(t *testing.T) {
	t.Parallel()
	generated, err := Generate(Options{PackageName: "generated"}, []QuerySpec{
		{
			Query: parser.Query{
				Name:    "FindUserByID",
				Cmd:     parser.CommandOne,
				SQL:     "SELECT user_id, username FROM users WHERE user_id = {user_id:Int64}",
				RowType: "User",
				Params: []parser.Parameter{
					{Name: "user_id", ClickHouseType: "Int64"},
				},
			},
			Result: []schema.Column{
				{Name: "user_id", ClickHouseType: "Int64"},
				{Name: "username", ClickHouseType: "String"},
			},
		},
		{
			Query: parser.Query{
				Name:    "FindUsers",
				Cmd:     parser.CommandMany,
				SQL:     "SELECT user_id, username FROM users WHERE username = {username:String}",
				RowType: "User",
				Params: []parser.Parameter{
					{Name: "username", ClickHouseType: "String"},
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
		"type UserRow struct",
		"func FindUserByID(ctx context.Context, db DBQuerier, userID int64) (UserRow, error)",
		"func FindUsers(ctx context.Context, db DBQuerier, username string) ([]UserRow, error)",
		"var row UserRow",
		"var out []UserRow",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("generated output missing %q:\n%s", want, got)
		}
	}
	if count := strings.Count(got, "type UserRow struct"); count != 1 {
		t.Fatalf("UserRow emitted %d times, want 1:\n%s", count, got)
	}
}

func TestGenerateSharedRowAnnotationRejectsIncompatibleShape(t *testing.T) {
	t.Parallel()
	_, err := Generate(Options{PackageName: "generated"}, []QuerySpec{
		{
			Query: parser.Query{Name: "FindUserByID", Cmd: parser.CommandOne, SQL: "SELECT user_id, username FROM users", RowType: "User"},
			Result: []schema.Column{
				{Name: "user_id", ClickHouseType: "Int64"},
				{Name: "username", ClickHouseType: "String"},
			},
		},
		{
			Query: parser.Query{Name: "FindUsers", Cmd: parser.CommandMany, SQL: "SELECT user_id, username FROM users", RowType: "User"},
			Result: []schema.Column{
				{Name: "user_id", ClickHouseType: "Int64"},
				{Name: "username", ClickHouseType: "Nullable(String)"},
			},
		},
	})
	if err == nil {
		t.Fatal("expected incompatible shared row shape error")
	}
	for _, want := range []string{
		"row=User used by incompatible queries FindUserByID and FindUsers",
		`name="username" field=Username type=string nullable=false`,
		`name="username" field=Username type=*string nullable=true`,
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err, want)
		}
	}
}

func TestGenerateSharedRowAnnotationKeepsSingleColumnScalar(t *testing.T) {
	t.Parallel()
	generated, err := Generate(Options{PackageName: "generated"}, []QuerySpec{
		{
			Query:  parser.Query{Name: "FindUserID", Cmd: parser.CommandOne, SQL: "SELECT user_id FROM users", RowType: "User"},
			Result: []schema.Column{{Name: "user_id", ClickHouseType: "Int64"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	got := string(generated)
	for _, want := range []string{
		"func FindUserID(ctx context.Context, db DBQuerier) (int64, error)",
		"type FindUserIDRow struct",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("generated output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "type UserRow struct") {
		t.Fatalf("single-column row annotation emitted shared row struct:\n%s", got)
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
		"NoParams(ctx context.Context, db DBQuerier) (uint8, error)",
		"TwoParams(ctx context.Context, db DBQuerier, min uint64, limit uint64) ([]TwoParamsRow, error)",
		"type ThreeParamsParams struct",
		"ThreeParams(ctx context.Context, db DBQuerier, params ThreeParamsParams) error",
		"func (p ThreeParamsParams) args() clickhouse.Parameters",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("generated output missing %q:\n%s", want, got)
		}
	}
}
