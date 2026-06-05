# chty-go

Go-native `chty` code generator for typed ClickHouse queries.

SQL files use sqlc-style annotations:

```sql
-- name: GetUser :one
SELECT user_id, username
FROM users
WHERE user_id = {user_id:Int64}
```

Generate Go wrappers:

```sh
go tool chty gen go --query-glob "queries/*.sql" --output-dir generated --package generated --db-url clickhouse://default@localhost:9000/default
```

Validate generated wrappers against live ClickHouse schema:

```sh
go tool chty validate --generated-glob "generated/*.go" --db-url clickhouse://default@localhost:9000/default
```

Downstream setup:

```sh
go get -tool github.com/meoyawn/chty-go/cmd/chty
```

Generation uses `github.com/ClickHouse/clickhouse-go/v2` native connections. Query parameters keep ClickHouse `{name:Type}` placeholders and are passed with `clickhouse.Named`.

## Related

- This is a source port of https://pypi.org/project/chty
- For Postgres SQL -> Go generator use https://github.com/meoyawn/pggen
