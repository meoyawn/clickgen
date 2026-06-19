package main

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestRunVersionSmoke(t *testing.T) {
	t.Parallel()
	assert.NilError(t, run([]string{"--version"}))
}
