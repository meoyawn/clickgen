# clickgen

ClickHouse SQL to Go code generator

Problem: Go row structs drift from ClickHouse SQL.

For example, when you edit a `SELECT` and add, remove, rename, or retype a
projected column, the Go row struct and scan code must change with it. clickgen
uses the query and a live ClickHouse schema to generate that Go wrapper, so the
query params and result projection stay in sync with Go field names and types.

```sql
-- name: GetUser :one
SELECT user_id, username
FROM users
WHERE user_id = {user_id:Int64}
```

## Install

```sh
go get -tool github.com/meoyawn/clickgen/cmd/clickgen
```

## Generate

Generate Go wrappers:

```sh
go tool clickgen gen go --query-glob "queries/*.sql" --output-dir generated --package generated --db-url clickhouse://default@localhost:9000/default
```

Validate generated wrappers against live ClickHouse schema:

```sh
go tool clickgen validate --generated-glob "generated/*.go" --db-url clickhouse://default@localhost:9000/default
```

Generation uses `github.com/ClickHouse/clickhouse-go/v2` native connections. Query parameters keep ClickHouse `{name:Type}` placeholders and are passed with `clickhouse.Named`.

## Related

- This is a source port of https://pypi.org/project/chty
- For Postgres SQL -> Go generator use https://github.com/meoyawn/pggen
