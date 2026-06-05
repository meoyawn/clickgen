package schema

import "testing"

func TestReplaceParametersWithDefaults(t *testing.T) {
	t.Parallel()
	query := "SELECT {id:Int32}, {name:String}, {tags:Array(String)}, {score:Nullable(Float64)}"
	got := ReplaceParametersWithDefaults(query)
	want := "SELECT 0, '', [], 0.0"
	if got != want {
		t.Fatalf("ReplaceParametersWithDefaults = %q, want %q", got, want)
	}
}
