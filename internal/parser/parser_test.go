package parser

import (
	"testing"

	"gotest.tools/v3/assert"
)

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
	assert.Equal(t, len(params), 2)
	assert.DeepEqual(t, params[0], Parameter{Name: "min_age", ClickHouseType: "Int32"})
	assert.DeepEqual(t, params[1], Parameter{Name: "tags", ClickHouseType: "Array(String)"})
}

func TestExtractParametersRejectsConflictingDuplicateTypes(t *testing.T) {
	t.Parallel()
	_, err := ExtractParametersStrict("SELECT {id:Int32}, {id:String}")
	assert.Assert(t, err != nil, "expected conflicting duplicate parameter error")
}

func TestExtractParametersRejectsLegacyBindPlaceholders(t *testing.T) {
	t.Parallel()
	for _, query := range []string{
		"SELECT * FROM users WHERE id = @id",
		"SELECT * FROM users WHERE id = ?",
		"SELECT * FROM users WHERE id = $1",
	} {
		_, err := ExtractParametersStrict(query)
		assert.Assert(t, err != nil, "expected legacy bind error for %q", query)
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
	assert.NilError(t, err)
	assert.DeepEqual(t, params, []Parameter{{Name: "id", ClickHouseType: "UInt64"}})
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
	assert.NilError(t, err)
	assert.Equal(t, len(queries), 3)
	assert.Equal(t, queries[0].Name, "GetUser")
	assert.Equal(t, queries[0].Cmd, CommandOne)
	assert.Equal(t, queries[0].RowType, "User")
	assert.Equal(t, queries[1].Name, "SearchUsers")
	assert.Equal(t, queries[1].Cmd, CommandMany)
	assert.Equal(t, queries[1].RowType, "")
	assert.Equal(t, queries[2].Name, "InsertUser")
	assert.Equal(t, queries[2].Cmd, CommandExec)
}

func TestParseRequiresAnnotations(t *testing.T) {
	t.Parallel()
	_, err := ParseSQL("bad.sql", "SELECT 1")
	assert.Assert(t, err != nil, "expected missing annotation error")
}
