package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/meoyawn/chty-go/internal/chtype"
	"github.com/meoyawn/chty-go/internal/codegen"
	"github.com/meoyawn/chty-go/internal/parser"
	"github.com/meoyawn/chty-go/internal/schema"
	"github.com/meoyawn/chty-go/internal/validator"
)

const version = "0.1.0"

type repeatedFlag []string

func (f *repeatedFlag) String() string {
	return fmt.Sprint([]string(*f))
}

func (f *repeatedFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printUsage(os.Stderr)
		return fmt.Errorf("missing command")
	}
	switch args[0] {
	case "--help", "-h", "help":
		printUsage(os.Stdout)
		return nil
	case "--version", "-v", "version":
		fmt.Println("chty version", version)
		return nil
	case "gen":
		if len(args) < 2 || args[1] != "go" {
			return fmt.Errorf("expected: chty gen go")
		}
		return runGenGo(args[2:])
	case "validate":
		return runValidate(args[1:])
	default:
		printUsage(os.Stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runGenGo(args []string) error {
	flags := flag.NewFlagSet("chty gen go", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	var queryGlobs repeatedFlag
	var goTypes repeatedFlag
	outputDir := flags.String("output-dir", "generated", "directory for generated Go files")
	packageName := flags.String("package", "generated", "generated Go package name")
	dbURL := flags.String("db-url", "", "ClickHouse native URL")
	flags.Var(&queryGlobs, "query-glob", "SQL query glob; may be repeated")
	flags.Var(&goTypes, "go-type", "override ClickHouseType=GoType; may be repeated")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if len(queryGlobs) == 0 {
		return fmt.Errorf("--query-glob is required")
	}

	overrides, err := parseOverrides(goTypes)
	if err != nil {
		return err
	}
	paths, err := expandGlobs(queryGlobs)
	if err != nil {
		return err
	}
	queries, err := parser.ParseFiles(paths)
	if err != nil {
		return err
	}

	specs := make([]codegen.QuerySpec, 0, len(queries))
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	var conn driver.Conn
	if *dbURL != "" {
		opened, err := schema.Open(*dbURL)
		if err != nil {
			return err
		}
		if err := opened.Ping(ctx); err != nil {
			return fmt.Errorf("ping ClickHouse: %w", err)
		}
		conn = opened
	}

	for _, query := range queries {
		spec := codegen.QuerySpec{Query: query}
		if query.Cmd != parser.CommandExec {
			if conn == nil {
				return fmt.Errorf("--db-url is required for query %s :%s result schema", query.Name, query.Cmd)
			}
			result, err := schema.Describe(ctx, conn, query.SQL)
			if err != nil {
				return fmt.Errorf("describe %s: %w", query.Name, err)
			}
			spec.Result = result
		}
		specs = append(specs, spec)
	}

	generated, err := codegen.Generate(codegen.Options{
		PackageName: *packageName,
		Overrides:   overrides,
	}, specs)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(*outputDir, 0o755); err != nil {
		return err
	}
	outputPath := filepath.Join(*outputDir, "chty_gen.go")
	if err := os.WriteFile(outputPath, generated, 0o644); err != nil {
		return err
	}
	fmt.Println(outputPath)
	return nil
}

func runValidate(args []string) error {
	flags := flag.NewFlagSet("chty validate", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	var generatedGlobs repeatedFlag
	var goTypes repeatedFlag
	dbURL := flags.String("db-url", "", "ClickHouse native URL")
	flags.Var(&generatedGlobs, "generated-glob", "generated Go glob; may be repeated")
	flags.Var(&goTypes, "go-type", "override ClickHouseType=GoType; may be repeated")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if len(generatedGlobs) == 0 {
		return fmt.Errorf("--generated-glob is required")
	}
	if *dbURL == "" {
		return fmt.Errorf("--db-url is required")
	}

	overrides, err := parseOverrides(goTypes)
	if err != nil {
		return err
	}
	paths, err := expandGlobs(generatedGlobs)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	allValid := true
	for _, path := range paths {
		valid, errors := validator.ValidateFile(ctx, path, *dbURL, overrides)
		if valid {
			fmt.Println(path)
			continue
		}
		allValid = false
		fmt.Fprintf(os.Stderr, "%s:\n", path)
		for _, errText := range errors {
			fmt.Fprintf(os.Stderr, "  - %s\n", errText)
		}
	}
	if !allValid {
		return fmt.Errorf("validation failed")
	}
	return nil
}

func parseOverrides(values []string) (chtype.Overrides, error) {
	overrides := chtype.Overrides{}
	for _, value := range values {
		chType, goType, err := chtype.ParseOverride(value)
		if err != nil {
			return nil, err
		}
		overrides.Add(chType, goType)
	}
	return overrides, nil
}

func expandGlobs(globs []string) ([]string, error) {
	seen := map[string]struct{}{}
	var paths []string
	for _, pattern := range globs {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, err
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("glob %q matched no files", pattern)
		}
		for _, match := range matches {
			if _, ok := seen[match]; ok {
				continue
			}
			seen[match] = struct{}{}
			paths = append(paths, match)
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func printUsage(out *os.File) {
	fmt.Fprintln(out, "Usage:")
	fmt.Fprintln(out, "  chty gen go --query-glob 'queries/*.sql' --output-dir generated --package generated --db-url clickhouse://default@localhost:9000/default")
	fmt.Fprintln(out, "  chty validate --generated-glob 'generated/*.go' --db-url clickhouse://default@localhost:9000/default")
}
