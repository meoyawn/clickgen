package parser

import "testing"

func TestExtractParameters(t *testing.T) {
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
	_, err := ExtractParametersStrict("SELECT {id:Int32}, {id:String}")
	if err == nil {
		t.Fatal("expected conflicting duplicate parameter error")
	}
}

func TestParseAnnotatedSQL(t *testing.T) {
	queries, err := ParseSQL("queries.sql", `
-- name: GetUser :one
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
	if queries[1].Name != "SearchUsers" || queries[1].Cmd != CommandMany {
		t.Fatalf("queries[1] = %#v", queries[1])
	}
	if queries[2].Name != "InsertUser" || queries[2].Cmd != CommandExec {
		t.Fatalf("queries[2] = %#v", queries[2])
	}
}

func TestParseRequiresAnnotations(t *testing.T) {
	_, err := ParseSQL("bad.sql", "SELECT 1")
	if err == nil {
		t.Fatal("expected missing annotation error")
	}
}
