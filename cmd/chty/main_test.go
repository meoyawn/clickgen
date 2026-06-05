package main

import "testing"

func TestRunVersionSmoke(t *testing.T) {
	if err := run([]string{"--version"}); err != nil {
		t.Fatal(err)
	}
}
