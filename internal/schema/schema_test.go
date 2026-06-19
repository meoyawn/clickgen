package schema

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestReplaceParametersWithDefaults(t *testing.T) {
	t.Parallel()
	query := "SELECT {id:Int32}, {name:String}, {tags:Array(String)}, {score:Nullable(Float64)}"
	got := ReplaceParametersWithDefaults(query)
	want := "SELECT 0, '', [], 0.0"
	assert.Equal(t, got, want)
}
