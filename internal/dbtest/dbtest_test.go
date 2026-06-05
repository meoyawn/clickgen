package dbtest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/meoyawn/chty-go/internal/codegen"
	fixture "github.com/meoyawn/chty-go/internal/dbtest/generated"
	"github.com/meoyawn/chty-go/internal/parser"
	"github.com/meoyawn/chty-go/internal/schema"
	"github.com/meoyawn/chty-go/internal/validator"
	"github.com/ory/dockertest/v4"
)

var (
	testDBURL     string
	testConn      driver.Conn
	dbUnavailable string
)

func TestMain(m *testing.M) {
	ctx := context.Background()
	pool, err := dockertest.NewPool(ctx, "", dockertest.WithMaxWait(90*time.Second))
	if err != nil {
		dbUnavailable = err.Error()
		os.Exit(m.Run())
	}

	resource, err := pool.Run(ctx, "clickhouse/clickhouse-server",
		dockertest.WithTag("26.3-alpine"),
		dockertest.WithoutReuse(),
	)
	if err != nil {
		dbUnavailable = err.Error()
		_ = pool.Close(ctx)
		os.Exit(m.Run())
	}

	hostPort := resource.GetPort("9000/tcp")
	testDBURL = fmt.Sprintf("clickhouse://default@localhost:%s/default", hostPort)
	if err := pool.Retry(ctx, 90*time.Second, func() error {
		conn, err := schema.Open(testDBURL)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := conn.Ping(ctx); err != nil {
			return err
		}
		testConn = conn
		return nil
	}); err != nil {
		dbUnavailable = err.Error()
		_ = pool.Close(ctx)
		os.Exit(m.Run())
	}

	if err := setupFixture(); err != nil {
		dbUnavailable = err.Error()
		_ = pool.Close(ctx)
		os.Exit(m.Run())
	}

	code := m.Run()
	if testConn != nil {
		_ = testConn.Close()
	}
	_ = pool.Close(ctx)
	os.Exit(code)
}

func setupFixture() error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	statements := []string{
		"DROP TABLE IF EXISTS chty_users",
		"CREATE TABLE chty_users (user_id Int64, username String, age Int32) ENGINE = Memory",
	}
	for _, stmt := range statements {
		if err := testConn.Exec(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func requireDB(t *testing.T) driver.Conn {
	t.Helper()
	if dbUnavailable != "" {
		t.Skipf("ClickHouse unavailable: %s", dbUnavailable)
	}
	return testConn
}

func TestDescribe(t *testing.T) {
	conn := requireDB(t)
	ctx := context.Background()
	columns, err := schema.Describe(ctx, conn, "SELECT number, number * 2 AS doubled FROM system.numbers LIMIT 1")
	if err != nil {
		t.Fatal(err)
	}
	if len(columns) != 2 {
		t.Fatalf("len(columns) = %d, want 2: %#v", len(columns), columns)
	}
	if columns[0].Name != "number" || columns[0].ClickHouseType != "UInt64" {
		t.Fatalf("columns[0] = %#v", columns[0])
	}
	if columns[1].Name != "doubled" || columns[1].ClickHouseType != "UInt64" {
		t.Fatalf("columns[1] = %#v", columns[1])
	}
}

func TestGeneratedExecution(t *testing.T) {
	conn := requireDB(t)
	ctx := context.Background()
	queries := fixture.NewQuerier(conn)

	number, err := queries.GetNumber(ctx, 7)
	if err != nil {
		t.Fatal(err)
	}
	if number != 7 {
		t.Fatalf("GetNumber = %d, want 7", number)
	}

	rows, err := queries.ListNumbers(ctx, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("len(rows) = %d, want 3", len(rows))
	}
	if rows[2].Number != 2 || rows[2].Doubled != 4 {
		t.Fatalf("rows[2] = %#v", rows[2])
	}

	if err := queries.InsertUser(ctx, fixture.InsertUserParams{UserID: 1, Username: "ada", Age: 37}); err != nil {
		t.Fatal(err)
	}
	var count uint64
	if err := conn.QueryRow(ctx, "SELECT count() FROM chty_users WHERE user_id = 1").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("inserted count = %d, want 1", count)
	}
}

func TestSchemaDriftValidation(t *testing.T) {
	requireDB(t)
	generated, err := codegen.Generate(codegen.Options{PackageName: "generated"}, []codegen.QuerySpec{
		{
			Query: parser.Query{
				Name: "Numbers",
				Cmd:  parser.CommandMany,
				SQL:  "SELECT number FROM system.numbers LIMIT 1",
			},
			Result: []schema.Column{{Name: "number", ClickHouseType: "String"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "chty_gen.go")
	if err := os.WriteFile(path, generated, 0o644); err != nil {
		t.Fatal(err)
	}

	valid, errors := validator.ValidateFile(context.Background(), path, testDBURL, nil)
	if valid {
		t.Fatal("ValidateFile returned valid, want drift error")
	}
	if !strings.Contains(strings.Join(errors, "\n"), "Result type mismatch for 'number'") {
		t.Fatalf("errors = %#v", errors)
	}
}
