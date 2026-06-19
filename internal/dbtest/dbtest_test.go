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
	"github.com/meoyawn/clickgen/internal/codegen"
	fixture "github.com/meoyawn/clickgen/internal/dbtest/generated"
	"github.com/meoyawn/clickgen/internal/parser"
	"github.com/meoyawn/clickgen/internal/schema"
	"github.com/meoyawn/clickgen/internal/validator"
	mobyclient "github.com/moby/moby/client"
	"github.com/ory/dockertest/v4"
	"gotest.tools/v3/assert"
)

const (
	clickHouseImage       = "clickhouse/clickhouse-server"
	clickHouseTag         = "26.3-alpine"
	clickHouseUser        = "admin"
	clickHousePassword    = "admin"
	dbtestPackageTimeout  = 10 * time.Second
	dbtestCleanupTimeout  = 5 * time.Second
	dbtestContainerName   = "clickgen-dbtest-clickhouse"
	dbtestContainerLabel  = "github.com/meoyawn/clickgen.dbtest"
	dbtestContainerLabelV = "true"
)

var (
	testDBURL     string
	testConn      driver.Conn
	dbUnavailable string
)

func TestMain(m *testing.M) {
	os.Exit(testMain(m))
}

func testMain(m *testing.M) int {
	ctx, cancel := context.WithTimeout(context.Background(), dbtestPackageTimeout)
	defer cancel()
	pool, err := dockertest.NewPool(ctx, "", dockertest.WithMaxWait(dbtestPackageTimeout))
	if err != nil {
		dbUnavailable = err.Error()
		return m.Run()
	}
	defer closePool(pool)

	if err := removeDBTestContainers(ctx, pool); err != nil {
		dbUnavailable = err.Error()
		return m.Run()
	}

	resource, err := pool.Run(ctx, clickHouseImage,
		dockertest.WithTag(clickHouseTag),
		dockertest.WithName(dbtestContainerName),
		dockertest.WithLabels(map[string]string{
			dbtestContainerLabel: dbtestContainerLabelV,
		}),
		dockertest.WithEnv([]string{
			"CLICKHOUSE_USER=" + clickHouseUser,
			"CLICKHOUSE_PASSWORD=" + clickHousePassword,
		}),
		dockertest.WithoutReuse(),
	)
	if err != nil {
		dbUnavailable = err.Error()
		return m.Run()
	}
	defer closeResource(pool, resource)

	hostPort := resource.GetHostPort("9000/tcp")
	testDBURL = fmt.Sprintf("clickhouse://%s:%s@%s/default", clickHouseUser, clickHousePassword, hostPort)
	if err := pool.Retry(ctx, dbtestPackageTimeout, func() error {
		conn, err := schema.Open(testDBURL)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), dbtestCleanupTimeout)
		defer cancel()
		if err := conn.Ping(ctx); err != nil {
			_ = conn.Close()
			return err
		}
		testConn = conn
		return nil
	}); err != nil {
		dbUnavailable = describeResourceFailure(resource, err)
		return m.Run()
	}

	if err := setupFixture(); err != nil {
		dbUnavailable = err.Error()
		return m.Run()
	}

	code := m.Run()
	if testConn != nil {
		_ = testConn.Close()
	}
	return code
}

func closePool(pool dockertest.ClosablePool) {
	ctx, cancel := context.WithTimeout(context.Background(), dbtestCleanupTimeout)
	defer cancel()
	_ = pool.Close(ctx)
}

func closeResource(pool dockertest.Pool, resource dockertest.ClosableResource) {
	ctx, cancel := context.WithTimeout(context.Background(), dbtestCleanupTimeout)
	defer cancel()
	_, _ = pool.Client().ContainerRemove(ctx, resource.ID(), mobyclient.ContainerRemoveOptions{
		Force:         true,
		RemoveVolumes: true,
	})
}

func describeResourceFailure(resource dockertest.Resource, err error) string {
	ctx, cancel := context.WithTimeout(context.Background(), dbtestCleanupTimeout)
	defer cancel()
	stdout, stderr, logsErr := resource.Logs(ctx)
	if logsErr != nil {
		return err.Error()
	}

	parts := []string{err.Error()}
	if stdout = strings.TrimSpace(stdout); stdout != "" {
		parts = append(parts, "ClickHouse stdout:\n"+tail(stdout, 4000))
	}
	if stderr = strings.TrimSpace(stderr); stderr != "" {
		parts = append(parts, "ClickHouse stderr:\n"+tail(stderr, 4000))
	}
	return strings.Join(parts, "\n")
}

func tail(value string, maxLen int) string {
	if len(value) <= maxLen {
		return value
	}
	return value[len(value)-maxLen:]
}

func removeDBTestContainers(ctx context.Context, pool dockertest.Pool) error {
	containers, err := pool.Client().ContainerList(ctx, mobyclient.ContainerListOptions{
		All: true,
		Filters: make(mobyclient.Filters).
			Add("name", dbtestContainerName).
			Add("label", dbtestContainerLabel+"="+dbtestContainerLabelV),
	})
	if err != nil {
		return fmt.Errorf("list stale dbtest containers: %w", err)
	}
	for _, container := range containers.Items {
		if _, err := pool.Client().ContainerRemove(ctx, container.ID, mobyclient.ContainerRemoveOptions{
			Force:         true,
			RemoveVolumes: true,
		}); err != nil {
			return fmt.Errorf("remove stale dbtest container %s: %w", container.ID, err)
		}
	}
	return nil
}

func setupFixture() error {
	ctx, cancel := context.WithTimeout(context.Background(), dbtestCleanupTimeout)
	defer cancel()
	statements := []string{
		"DROP TABLE IF EXISTS clickgen_users",
		"CREATE TABLE clickgen_users (user_id Int64, username String, age Int32) ENGINE = Memory",
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
	conn, err := schema.Open(testDBURL)
	assert.NilError(t, err)
	t.Cleanup(func() {
		_ = conn.Close()
	})
	return conn
}

func TestDescribe(t *testing.T) {
	t.Parallel()
	conn := requireDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), dbtestCleanupTimeout)
	defer cancel()
	columns, err := schema.Describe(ctx, conn, "SELECT number, number * 2 AS doubled FROM system.numbers LIMIT 1")
	assert.NilError(t, err)
	assert.Equal(t, len(columns), 2, "columns: %#v", columns)
	assert.Equal(t, columns[0].Name, "number")
	assert.Equal(t, columns[0].ClickHouseType, "UInt64")
	assert.Equal(t, columns[1].Name, "doubled")
	assert.Equal(t, columns[1].ClickHouseType, "UInt64")
}

func TestGeneratedExecution(t *testing.T) {
	t.Parallel()
	conn := requireDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), dbtestCleanupTimeout)
	defer cancel()

	number, err := fixture.GetNumber(ctx, conn, 7)
	assert.NilError(t, err)
	assert.Equal(t, number, uint64(7))

	rows, err := fixture.ListNumbers(ctx, conn, 3)
	assert.NilError(t, err)
	assert.Equal(t, len(rows), 3)
	assert.Equal(t, rows[2].Number, uint64(2))
	assert.Equal(t, rows[2].Doubled, uint64(4))

	rangeRows, err := fixture.RangeNumbers(ctx, conn, 2, 5)
	assert.NilError(t, err)
	assert.Equal(t, len(rangeRows), 3)
	assert.Equal(t, rangeRows[0].Number, uint64(2))
	assert.Equal(t, rangeRows[2].Number, uint64(4))

	assert.NilError(t, fixture.InsertUser(ctx, conn, fixture.InsertUserParams{UserID: 1, Username: "ada", Age: 37}))
	var count uint64
	assert.NilError(t, conn.QueryRow(ctx, "SELECT count() FROM clickgen_users WHERE user_id = 1").Scan(&count))
	assert.Equal(t, count, uint64(1))
}

func TestSchemaDriftValidation(t *testing.T) {
	t.Parallel()
	requireDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), dbtestCleanupTimeout)
	defer cancel()
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
	assert.NilError(t, err)
	path := filepath.Join(t.TempDir(), "clickgen_gen.go")
	assert.NilError(t, os.WriteFile(path, generated, 0o644))

	valid, errors := validator.ValidateFile(ctx, path, testDBURL, nil)
	assert.Assert(t, !valid, "ValidateFile returned valid, want drift error")
	assert.Assert(t, strings.Contains(strings.Join(errors, "\n"), "Result type mismatch for 'number'"), "errors = %#v", errors)
}
