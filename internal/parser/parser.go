package parser

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

type Command string

const (
	CommandOne  Command = "one"
	CommandMany Command = "many"
	CommandExec Command = "exec"
)

type Parameter struct {
	Name           string
	ClickHouseType string
}

type Query struct {
	Name    string
	Cmd     Command
	SQL     string
	Params  []Parameter
	RowType string
	Source  string
	Line    int
}

var (
	annotationRE = regexp.MustCompile(`^\s*--\s*name:\s*([A-Za-z_][A-Za-z0-9_]*)\s+:(one|many|exec)(?:\s+(.*))?\s*$`)
	parameterRE  = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*):([^}]+)\}`)
)

func ParseFile(path string) ([]Query, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseSQL(path, string(content))
}

func ParseSQL(source, content string) ([]Query, error) {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")

	var queries []Query
	var current *Query
	var body []string
	var prelude []string

	flush := func(endLine int) error {
		if current == nil {
			return nil
		}
		sql := strings.TrimSpace(strings.Join(body, "\n"))
		if sql == "" {
			return fmt.Errorf("%s:%d: query %q has no SQL body", source, current.Line, current.Name)
		}
		params, err := ExtractParametersStrict(sql)
		if err != nil {
			return fmt.Errorf("%s:%d: %w", source, current.Line, err)
		}
		current.SQL = sql
		current.Params = params
		queries = append(queries, *current)
		body = nil
		_ = endLine
		return nil
	}

	for idx, line := range lines {
		lineNo := idx + 1
		if match := annotationRE.FindStringSubmatch(line); match != nil {
			if err := flush(lineNo); err != nil {
				return nil, err
			}
			rowType, err := parseAnnotationPragmas(match[3])
			if err != nil {
				return nil, fmt.Errorf("%s:%d: %w", source, lineNo, err)
			}
			current = &Query{
				Name:    match[1],
				Cmd:     Command(match[2]),
				RowType: rowType,
				Source:  source,
				Line:    lineNo,
			}
			body = nil
			continue
		}

		if current == nil {
			if strings.TrimSpace(line) != "" {
				prelude = append(prelude, line)
			}
			continue
		}
		body = append(body, line)
	}

	if err := flush(len(lines)); err != nil {
		return nil, err
	}
	if len(queries) == 0 {
		if len(prelude) > 0 {
			return nil, fmt.Errorf("%s: missing required annotation: -- name: QueryName :one|:many|:exec", source)
		}
		return nil, fmt.Errorf("%s: no queries found", source)
	}
	return queries, nil
}

func parseAnnotationPragmas(input string) (string, error) {
	var rowType string
	for pragma := range strings.FieldsSeq(input) {
		key, value, ok := strings.Cut(pragma, "=")
		if !ok {
			return "", fmt.Errorf("invalid pragma %q; expected key=value", pragma)
		}
		switch key {
		case "row":
			if value == "" {
				return "", fmt.Errorf("row pragma must not be empty")
			}
			rowType = value
		default:
			return "", fmt.Errorf("unsupported pragma %q", key)
		}
	}
	return rowType, nil
}

func ParseFiles(paths []string) ([]Query, error) {
	var out []Query
	seen := map[string]string{}
	for _, path := range paths {
		queries, err := ParseFile(path)
		if err != nil {
			return nil, err
		}
		for _, query := range queries {
			if prev, ok := seen[query.Name]; ok {
				return nil, fmt.Errorf("%s:%d: duplicate query name %q, already defined in %s", query.Source, query.Line, query.Name, prev)
			}
			seen[query.Name] = fmt.Sprintf("%s:%d", query.Source, query.Line)
			out = append(out, query)
		}
	}
	return out, nil
}

func ExtractParameters(query string) []Parameter {
	params, _ := ExtractParametersStrict(query)
	return params
}

func ExtractParametersStrict(query string) ([]Parameter, error) {
	if placeholder, ok := findLegacyBindPlaceholder(query); ok {
		return nil, fmt.Errorf("legacy bind placeholder %q is not supported; use {name:Type}", placeholder)
	}

	matches := parameterRE.FindAllStringSubmatch(query, -1)
	params := make([]Parameter, 0, len(matches))
	seen := map[string]string{}

	for _, match := range matches {
		name := match[1]
		chType := strings.TrimSpace(match[2])
		if prev, ok := seen[name]; ok {
			if prev != chType {
				return nil, fmt.Errorf("parameter %q has conflicting types %q and %q", name, prev, chType)
			}
			continue
		}
		seen[name] = chType
		params = append(params, Parameter{Name: name, ClickHouseType: chType})
	}

	return params, nil
}

func findLegacyBindPlaceholder(query string) (string, bool) {
	for idx := 0; idx < len(query); idx++ {
		switch query[idx] {
		case '\'', '"', '`':
			idx = skipQuoted(query, idx)
		case '-':
			if idx+1 < len(query) && query[idx+1] == '-' {
				idx = skipLineComment(query, idx+2)
			}
		case '/':
			if idx+1 < len(query) && query[idx+1] == '*' {
				idx = skipBlockComment(query, idx+2)
			}
		case '@':
			if idx+1 < len(query) && isBindNameChar(query[idx+1]) {
				end := idx + 2
				for end < len(query) && isBindNameChar(query[end]) {
					end++
				}
				return query[idx:end], true
			}
		case '?':
			return "?", true
		case '$':
			if idx+1 < len(query) && query[idx+1] >= '0' && query[idx+1] <= '9' {
				end := idx + 2
				for end < len(query) && query[end] >= '0' && query[end] <= '9' {
					end++
				}
				return query[idx:end], true
			}
		}
	}

	return "", false
}

func skipQuoted(query string, idx int) int {
	quote := query[idx]
	for idx++; idx < len(query); idx++ {
		if query[idx] == '\\' {
			idx++
			continue
		}
		if query[idx] != quote {
			continue
		}
		if quote == '\'' && idx+1 < len(query) && query[idx+1] == '\'' {
			idx++
			continue
		}
		return idx
	}
	return len(query) - 1
}

func skipLineComment(query string, idx int) int {
	for ; idx < len(query); idx++ {
		if query[idx] == '\n' {
			return idx
		}
	}
	return len(query) - 1
}

func skipBlockComment(query string, idx int) int {
	for ; idx+1 < len(query); idx++ {
		if query[idx] == '*' && query[idx+1] == '/' {
			return idx + 1
		}
	}
	return len(query) - 1
}

func isBindNameChar(value byte) bool {
	return (value >= 'A' && value <= 'Z') ||
		(value >= 'a' && value <= 'z') ||
		(value >= '0' && value <= '9') ||
		value == '_'
}
