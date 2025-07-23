package main

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

// postgresQueryRequest represents the expected body of the query request.
type postgresQueryRequest struct {
	ConnString string `json:"conn_string"`
	Query      string `json:"query"`
}

// querier defines the minimal Query method we use from pgx connections.
type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// connector represents the minimal functionality from pgx.Conn used by the handler.
type connector interface {
	querier
	Close(context.Context) error
}

// connectFunc allows tests to stub pgx.Connect.
var connectFunc = func(ctx context.Context, connStr string) (connector, error) {
	return pgx.Connect(ctx, connStr)
}

// postgresQueryHandler executes a SQL query against Postgres and returns the results.
func postgresQueryHandler(c echo.Context) error {
	var req postgresQueryRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}
	if req.Query == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Query is required"})
	}

	ctx := c.Request().Context()
	cfg := c.Get("config").(*Config)

	var (
		q       querier
		cleanup func()
		err     error
	)

	if req.ConnString == "" {
		if cfg.DBPool == nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Database connection pool not initialized"})
		}
		poolConn, err := cfg.DBPool.Acquire(ctx)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to acquire database connection"})
		}
		q = poolConn
		cleanup = poolConn.Release
	} else {
		var conn connector
		conn, err = connectFunc(ctx, req.ConnString)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to connect to database"})
		}
		q = conn
		cleanup = func() { conn.Close(ctx) }
	}

	if cleanup != nil {
		defer cleanup()
	}

	rows, err := q.Query(ctx, req.Query)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	defer rows.Close()

	results := []map[string]interface{}{}
	fieldDescs := rows.FieldDescriptions()
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		rowMap := make(map[string]interface{}, len(vals))
		for i, fd := range fieldDescs {
			rowMap[string(fd.Name)] = vals[i]
		}
		results = append(results, rowMap)
	}
	if rows.Err() != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": rows.Err().Error()})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"results": results})
}
