package validator

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/meoyawn/chty-go/internal/chtype"
	"github.com/meoyawn/chty-go/internal/parser"
	"github.com/meoyawn/chty-go/internal/schema"
)

type Parameter struct {
	Name           string
	ClickHouseType string
	GoType         string
}

type ResultColumn struct {
	Name           string
	ClickHouseType string
	GoType         string
}

type GeneratedQuery struct {
	Name    string
	Kind    parser.Command
	SQL     string
	Params  []Parameter
	Results []ResultColumn
}

func ExtractGeneratedQueries(path string) ([]GeneratedQuery, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var queries []GeneratedQuery
	indexByName := map[string]int{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "// chty:") {
			continue
		}
		parts := strings.Split(line, "\t")
		switch parts[0] {
		case "// chty:query":
			if len(parts) != 4 {
				return nil, fmt.Errorf("invalid query metadata line: %s", line)
			}
			sql, err := decode(parts[3])
			if err != nil {
				return nil, fmt.Errorf("decode query metadata for %s: %w", parts[1], err)
			}
			kind := parser.Command(parts[2])
			if kind != parser.CommandOne && kind != parser.CommandMany && kind != parser.CommandExec {
				return nil, fmt.Errorf("invalid query kind %q for %s", parts[2], parts[1])
			}
			indexByName[parts[1]] = len(queries)
			queries = append(queries, GeneratedQuery{Name: parts[1], Kind: kind, SQL: sql})
		case "// chty:param":
			if len(parts) != 5 {
				return nil, fmt.Errorf("invalid parameter metadata line: %s", line)
			}
			idx, ok := indexByName[parts[1]]
			if !ok {
				return nil, fmt.Errorf("parameter metadata references unknown query %s", parts[1])
			}
			chType, err := decode(parts[3])
			if err != nil {
				return nil, fmt.Errorf("decode parameter metadata for %s.%s: %w", parts[1], parts[2], err)
			}
			goType, err := decode(parts[4])
			if err != nil {
				return nil, fmt.Errorf("decode parameter metadata for %s.%s: %w", parts[1], parts[2], err)
			}
			queries[idx].Params = append(queries[idx].Params, Parameter{
				Name:           parts[2],
				ClickHouseType: chType,
				GoType:         goType,
			})
		case "// chty:result":
			if len(parts) != 5 {
				return nil, fmt.Errorf("invalid result metadata line: %s", line)
			}
			idx, ok := indexByName[parts[1]]
			if !ok {
				return nil, fmt.Errorf("result metadata references unknown query %s", parts[1])
			}
			chType, err := decode(parts[3])
			if err != nil {
				return nil, fmt.Errorf("decode result metadata for %s.%s: %w", parts[1], parts[2], err)
			}
			goType, err := decode(parts[4])
			if err != nil {
				return nil, fmt.Errorf("decode result metadata for %s.%s: %w", parts[1], parts[2], err)
			}
			queries[idx].Results = append(queries[idx].Results, ResultColumn{
				Name:           parts[2],
				ClickHouseType: chType,
				GoType:         goType,
			})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(queries) == 0 {
		return nil, fmt.Errorf("could not find chty metadata in %s", path)
	}
	return queries, nil
}

func ValidateFile(ctx context.Context, path, dbURL string, overrides chtype.Overrides) (bool, []string) {
	queries, err := ExtractGeneratedQueries(path)
	if err != nil {
		return false, []string{"Validation error: " + err.Error()}
	}

	conn, err := schema.Open(dbURL)
	if err != nil {
		return false, []string{"Failed to connect to ClickHouse: " + err.Error()}
	}
	return ValidateQueries(ctx, conn, queries, overrides)
}

func ValidateQueries(ctx context.Context, conn driver.Conn, queries []GeneratedQuery, overrides chtype.Overrides) (bool, []string) {
	var errors []string
	for _, query := range queries {
		errors = append(errors, validateParameters(query, overrides)...)
		if query.Kind == parser.CommandExec {
			continue
		}
		if len(query.Results) == 0 {
			errors = append(errors, fmt.Sprintf("No result schema found for %s. File was likely generated without --db-url.", query.Name))
			continue
		}
		currentSchema, err := schema.Describe(ctx, conn, query.SQL)
		if err != nil {
			errors = append(errors, "Failed to get schema from ClickHouse: "+err.Error())
			continue
		}
		errors = append(errors, validateResultSchema(query, currentSchema, overrides)...)
	}
	return len(errors) == 0, errors
}

func validateParameters(query GeneratedQuery, overrides chtype.Overrides) []string {
	var errors []string
	queryParams, err := parser.ExtractParametersStrict(query.SQL)
	if err != nil {
		return []string{err.Error()}
	}

	expected := map[string]Parameter{}
	for _, param := range query.Params {
		expected[param.Name] = param
	}
	queryParamNames := map[string]struct{}{}
	for _, param := range queryParams {
		queryParamNames[param.Name] = struct{}{}
	}

	missing := namesDifference(queryParamNames, paramNames(expected))
	if len(missing) > 0 {
		errors = append(errors, "Query parameter missing from generated parameters: "+strings.Join(missing, ", "))
	}
	extra := namesDifference(paramNames(expected), queryParamNames)
	if len(extra) > 0 {
		errors = append(errors, "Generated parameters has parameter not in query: "+strings.Join(extra, ", "))
	}

	for _, param := range queryParams {
		generated, ok := expected[param.Name]
		if !ok {
			continue
		}
		actualGoType := chtype.Map(param.ClickHouseType, overrides).Name
		if generated.GoType != actualGoType {
			errors = append(errors, fmt.Sprintf(
				"Parameter type mismatch for '%s': query expects %s, generated parameters has %s",
				param.Name,
				actualGoType,
				generated.GoType,
			))
		}
	}
	return errors
}

func validateResultSchema(query GeneratedQuery, actual []schema.Column, overrides chtype.Overrides) []string {
	var errors []string
	expected := map[string]ResultColumn{}
	for _, col := range query.Results {
		expected[col.Name] = col
	}
	actualByName := map[string]schema.Column{}
	for _, col := range actual {
		actualByName[col.Name] = col
	}

	missing := namesDifference(resultNames(expected), columnNames(actualByName))
	if len(missing) > 0 {
		errors = append(errors, "Generated result expects column not in query result: "+strings.Join(missing, ", "))
	}
	extra := namesDifference(columnNames(actualByName), resultNames(expected))
	if len(extra) > 0 {
		errors = append(errors, "Query result has column not in generated result: "+strings.Join(extra, ", "))
	}

	for name, generated := range expected {
		actualCol, ok := actualByName[name]
		if !ok {
			continue
		}
		actualGoType := chtype.Map(actualCol.ClickHouseType, overrides).Name
		if generated.GoType != actualGoType {
			errors = append(errors, fmt.Sprintf(
				"Result type mismatch for '%s': query returns %s, generated result expects %s",
				name,
				actualGoType,
				generated.GoType,
			))
		}
	}
	sort.Strings(errors)
	return errors
}

func decode(value string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

func paramNames(input map[string]Parameter) map[string]struct{} {
	out := map[string]struct{}{}
	for name := range input {
		out[name] = struct{}{}
	}
	return out
}

func resultNames(input map[string]ResultColumn) map[string]struct{} {
	out := map[string]struct{}{}
	for name := range input {
		out[name] = struct{}{}
	}
	return out
}

func columnNames(input map[string]schema.Column) map[string]struct{} {
	out := map[string]struct{}{}
	for name := range input {
		out[name] = struct{}{}
	}
	return out
}

func namesDifference(left, right map[string]struct{}) []string {
	var out []string
	for name := range left {
		if _, ok := right[name]; !ok {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}
