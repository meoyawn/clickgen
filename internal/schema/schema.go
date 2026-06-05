package schema

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/meoyawn/chty-go/internal/chtype"
)

type Column struct {
	Name           string
	ClickHouseType string
}

var parameterRE = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*):([^}]+)\}`)

func Open(dbURL string) (driver.Conn, error) {
	parsed, err := url.Parse(dbURL)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme == "" {
		return nil, fmt.Errorf("invalid db-url: missing scheme")
	}

	secure := false
	switch parsed.Scheme {
	case "clickhouse", "clickhouse+native", "tcp":
	case "https":
		secure = true
	case "http":
	default:
		return nil, fmt.Errorf("invalid db-url scheme %q: expected clickhouse, clickhouse+native, tcp, http, or https", parsed.Scheme)
	}
	if value := parsed.Query().Get("secure"); value == "true" || value == "1" {
		secure = true
	}

	host := parsed.Hostname()
	if host == "" {
		host = "localhost"
	}
	port := parsed.Port()
	if port == "" {
		if secure {
			port = "9440"
		} else {
			port = "9000"
		}
	}
	username := parsed.User.Username()
	if username == "" {
		username = "default"
	}
	password, _ := parsed.User.Password()
	database := strings.Trim(strings.TrimPrefix(parsed.Path, "/"), "/")
	if database == "" {
		database = "default"
	}

	var tlsConfig *tls.Config
	if secure {
		tlsConfig = &tls.Config{ServerName: host}
		if value := parsed.Query().Get("skip_verify"); value == "true" || value == "1" {
			tlsConfig.InsecureSkipVerify = true
		}
	}

	return clickhouse.Open(&clickhouse.Options{
		Addr: []string{net.JoinHostPort(host, port)},
		Auth: clickhouse.Auth{
			Database: database,
			Username: username,
			Password: password,
		},
		TLS: tlsConfig,
	})
}

func GetResultSchema(ctx context.Context, query, dbURL string) ([]Column, error) {
	conn, err := Open(dbURL)
	if err != nil {
		return nil, err
	}
	return Describe(ctx, conn, query)
}

func Describe(ctx context.Context, conn driver.Conn, query string) ([]Column, error) {
	safeQuery := ReplaceParametersWithDefaults(query)
	safeQuery = strings.TrimSpace(safeQuery)
	safeQuery = strings.TrimSuffix(safeQuery, ";")
	rows, err := conn.Query(ctx, "DESCRIBE TABLE ("+safeQuery+")")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []Column
	for rows.Next() {
		names := rows.Columns()
		values := make([]string, len(names))
		dest := make([]any, len(names))
		for idx := range values {
			dest[idx] = &values[idx]
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		if len(values) < 2 {
			return nil, fmt.Errorf("DESCRIBE returned %d columns, expected at least 2", len(values))
		}
		columns = append(columns, Column{
			Name:           values[0],
			ClickHouseType: values[1],
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return columns, nil
}

func ReplaceParametersWithDefaults(query string) string {
	return parameterRE.ReplaceAllStringFunc(query, func(match string) string {
		parts := parameterRE.FindStringSubmatch(match)
		if parts == nil {
			return match
		}
		return chtype.DefaultLiteral(parts[2])
	})
}
