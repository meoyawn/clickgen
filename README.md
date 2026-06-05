# clickgen

ClickHouse SQL to Go code generator

Problem: keep ClickHouse query params and result projections in sync with Go structs, including field names and types.

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
