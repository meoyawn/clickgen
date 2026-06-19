package parser

import "testing"

func TestExtractParameters(t *testing.T) {
	t.Parallel()
	query := `
SELECT *
FROM users
WHERE age >= {min_age:Int32}
  AND tags IN {tags:Array(String)}
  AND age <= {min_age:Int32}
`
	params := ExtractParameters(query)
	if len(params) != 2 {
		t.Fatalf("len(params) = %d, want 2", len(params))
	}
	if params[0].Name != "min_age" || params[0].ClickHouseType != "Int32" {
		t.Fatalf("params[0] = %#v", params[0])
	}
	if params[1].Name != "tags" || params[1].ClickHouseType != "Array(String)" {
		t.Fatalf("params[1] = %#v", params[1])
	}
}

func TestExtractParametersRejectsConflictingDuplicateTypes(t *testing.T) {
	t.Parallel()
	_, err := ExtractParametersStrict("SELECT {id:Int32}, {id:String}")
	if err == nil {
		t.Fatal("expected conflicting duplicate parameter error")
	}
}

func TestExtractParametersRejectsLegacyBindPlaceholders(t *testing.T) {
	t.Parallel()
	for _, query := range []string{
		"SELECT * FROM users WHERE id = @id",
		"SELECT * FROM users WHERE id = ?",
		"SELECT * FROM users WHERE id = $1",
	} {
		_, err := ExtractParametersStrict(query)
		if err == nil {
			t.Fatalf("expected legacy bind error for %q", query)
		}
	}
}

func TestExtractParametersAllowsLegacyBindTokensInLiteralsAndComments(t *testing.T) {
	t.Parallel()
	query := `
SELECT *
FROM users
WHERE email = 'admin@example.com'
  AND note = '?'
  AND dollar = '$1'
  AND id = {id:UInt64}
-- ignored = @id
/* ignored = ? */
`
	params, err := ExtractParametersStrict(query)
	if err != nil {
		t.Fatal(err)
	}
	if len(params) != 1 || params[0].Name != "id" {
		t.Fatalf("params = %#v, want id only", params)
	}
}

func TestParseAnnotatedSQL(t *testing.T) {
	t.Parallel()
	queries, err := ParseSQL("queries.sql", `
-- name: GetUser :one row=User
SELECT user_id, username FROM users WHERE user_id = {user_id:Int64}

-- name: SearchUsers :many
SELECT user_id FROM users WHERE username LIKE {pattern:String}

-- name: InsertUser :exec
INSERT INTO users (user_id, username) VALUES ({user_id:Int64}, {username:String})
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(queries) != 3 {
		t.Fatalf("len(queries) = %d, want 3", len(queries))
	}
	if queries[0].Name != "GetUser" || queries[0].Cmd != CommandOne {
		t.Fatalf("queries[0] = %#v", queries[0])
	}
	if queries[0].RowType != "User" {
		t.Fatalf("queries[0].RowType = %q, want User", queries[0].RowType)
	}
	if queries[1].Name != "SearchUsers" || queries[1].Cmd != CommandMany {
		t.Fatalf("queries[1] = %#v", queries[1])
	}
	if queries[1].RowType != "" {
		t.Fatalf("queries[1].RowType = %q, want empty", queries[1].RowType)
	}
	if queries[2].Name != "InsertUser" || queries[2].Cmd != CommandExec {
		t.Fatalf("queries[2] = %#v", queries[2])
	}
}

func TestParseRequiresAnnotations(t *testing.T) {
	t.Parallel()
	_, err := ParseSQL("bad.sql", "SELECT 1")
	if err == nil {
		t.Fatal("expected missing annotation error")
	}
}
