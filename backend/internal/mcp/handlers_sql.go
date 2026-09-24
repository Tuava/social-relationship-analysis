package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var forbiddenSQLKeywords = regexp.MustCompile(`(?i)\b(INSERT|UPDATE|DELETE|DROP|ALTER|TRUNCATE|CREATE|GRANT|REVOKE|MERGE|CALL|DO|COPY|VACUUM|LOCK|REFRESH)\b`)
var forbiddenSQLRelations = regexp.MustCompile(`(?i)\b(system_configs|app_users|bot_instances|napcat_accounts|qzone_connections|source_connections)\b`)

func validateReadOnlySQL(query string) error {
	query = strings.TrimSpace(query)
	if query == "" {
		return fmt.Errorf("sql cannot be empty")
	}
	if strings.Contains(query, ";") {
		return fmt.Errorf("multiple SQL statements are not allowed")
	}
	fields := strings.Fields(query)
	if len(fields) == 0 || (strings.ToUpper(fields[0]) != "SELECT" && strings.ToUpper(fields[0]) != "WITH") {
		return fmt.Errorf("only SELECT or WITH queries are allowed")
	}
	if forbiddenSQLKeywords.MatchString(query) {
		return fmt.Errorf("mutating or administrative SQL is not allowed")
	}
	if forbiddenSQLRelations.MatchString(query) {
		return fmt.Errorf("credential and runtime configuration tables are not available through sra_sql_query")
	}
	return nil
}

func (h *HandlerRegistry) handleSQLQuery(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		SQL     string `json:"sql"`
		MaxRows int    `json:"max_rows"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("invalid arguments: " + err.Error()), nil
	}
	sql := strings.TrimSpace(input.SQL)
	if input.MaxRows <= 0 || input.MaxRows > 200 {
		input.MaxRows = 50
	}

	if err := validateReadOnlySQL(sql); err != nil {
		return errorResult(err.Error()), nil
	}

	// Arbitrary SQL never runs as the application owner.
	if h.SQLDB == nil {
		return errorResult("SQL tool requires a restricted SRA_SQL_DATABASE_URL; see docs/sql-reader.md"), nil
	}
	// Execute in a read-only transaction with timeout
	tx, err := h.SQLDB.Begin(ctx)
	if err != nil {
		return errorResult("failed to begin transaction: " + err.Error()), nil
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, "SET TRANSACTION READ ONLY")
	if err != nil {
		return errorResult("failed to set read-only: " + err.Error()), nil
	}
	_, err = tx.Exec(ctx, "SET LOCAL statement_timeout = '10s'")
	if err != nil {
		return errorResult("failed to set timeout: " + err.Error()), nil
	}

	// Wrapping avoids breaking a caller-provided ORDER BY/LIMIT and prevents
	// user text from becoming a second statement.
	boundedSQL := fmt.Sprintf("SELECT * FROM (%s) AS sra_query LIMIT %d", sql, input.MaxRows)
	rows, err := tx.Query(ctx, boundedSQL)
	if err != nil {
		return errorResult("query error: " + err.Error()), nil
	}
	defer rows.Close()

	// Get column names
	fieldDescriptions := rows.FieldDescriptions()
	columns := make([]string, len(fieldDescriptions))
	for i, fd := range fieldDescriptions {
		columns[i] = string(fd.Name)
	}

	// Read rows into generic maps
	var results []map[string]any
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return errorResult("row scan error: " + err.Error()), nil
		}
		row := make(map[string]any, len(columns))
		for i, col := range columns {
			v := values[i]
			// Convert time.Time to string for JSON
			if t, ok := v.(time.Time); ok {
				row[col] = t.Format(time.RFC3339)
			} else {
				row[col] = v
			}
		}
		results = append(results, row)
	}
	if rows.Err() != nil {
		return errorResult("query iteration error: " + rows.Err().Error()), nil
	}

	return jsonResult(map[string]any{
		"columns":   columns,
		"row_count": len(results),
		"rows":      results,
		"truncated": len(results) >= input.MaxRows,
	})
}
