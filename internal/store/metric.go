package store

import (
	"context"
	"database/sql/driver"
	"regexp"
	"strings"
	"time"

	"github.com/ngrok/sqlmw"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	opRegex     = regexp.MustCompile(`^(\w)+`)
	pgOpLatency *prometheus.HistogramVec
	pgOpTotal   *prometheus.CounterVec
)

type metricInterceptor struct {
	sqlmw.NullInterceptor
}

func init() {
	pgOpLatency = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:      "pg_op_duration_milliseconds",
		Help:      "Time spent on a postgres operation",
		Subsystem: "assisted_migrations",
		Buckets:   []float64{100, 300, 500, 1000, 5000},
	},
		[]string{"op", "method"},
	)
	pgOpTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name:      "pg_op_total",
		Help:      "Number of postgres operations",
		Subsystem: "assisted_migrations",
	},
		[]string{"op"},
	)

	prometheus.MustRegister(pgOpLatency)
	prometheus.MustRegister(pgOpTotal)
}

func (mi *metricInterceptor) ConnBeginTx(ctx context.Context, conn driver.ConnBeginTx, opts driver.TxOptions) (context.Context, driver.Tx, error) {
	start := time.Now()
	defer mi.measure("conn-begin-tx", "conn-begin-tx", start)

	tx, err := conn.BeginTx(ctx, opts)
	return ctx, tx, err
}

func (mi *metricInterceptor) ConnPrepareContext(ctx context.Context, conn driver.ConnPrepareContext, query string) (context.Context, driver.Stmt, error) {
	start := time.Now()
	defer mi.measure("conn-prepare-context", "conn-prepare-context", start)

	stmt, err := conn.PrepareContext(ctx, query)
	return ctx, stmt, err
}

func (mi *metricInterceptor) ConnPing(ctx context.Context, conn driver.Pinger) error {
	start := time.Now()
	defer mi.measure("conn-ping", "conn-ping", start)

	return conn.Ping(ctx)
}

func (mi *metricInterceptor) ConnExecContext(ctx context.Context, conn driver.ExecerContext, query string, args []driver.NamedValue) (driver.Result, error) {
	start := time.Now()
	matches := opRegex.FindSubmatch([]byte(query))
	method := "conn-exec-context"
	if len(matches) > 0 {
		method = string(matches[0])
	}
	defer mi.measure("conn-exec-context", strings.ToLower(method), start)

	return conn.ExecContext(ctx, query, args)
}

func (mi *metricInterceptor) ConnQueryContext(ctx context.Context, conn driver.QueryerContext, query string, args []driver.NamedValue) (context.Context, driver.Rows, error) {
	start := time.Now()
	method := "conn-query-context"
	matches := opRegex.FindSubmatch([]byte(query))
	if len(matches) > 0 {
		method = string(matches[0])
	}
	defer mi.measure("conn-exec-context", strings.ToLower(method), start)

	rows, err := conn.QueryContext(ctx, query, args)
	return ctx, rows, err
}

// Connector interceptors
func (mi *metricInterceptor) ConnectorConnect(ctx context.Context, conn driver.Connector) (driver.Conn, error) {
	start := time.Now()
	defer mi.measure("connector-connect", "connector-connect", start)
	return conn.Connect(ctx)
}

// Rows interceptors
func (mi *metricInterceptor) RowsNext(ctx context.Context, conn driver.Rows, dest []driver.Value) error {
	start := time.Now()
	defer mi.measure("rows-next", "rows-next", start)
	return conn.Next(dest)
}

// Stmt interceptors
func (mi *metricInterceptor) StmtExecContext(ctx context.Context, conn driver.StmtExecContext, _ string, args []driver.NamedValue) (driver.Result, error) {
	start := time.Now()
	defer mi.measure("stmt-exec-context", "stmt-exec-context", start)
	return conn.ExecContext(ctx, args)
}

func (mi *metricInterceptor) StmtQueryContext(ctx context.Context, conn driver.StmtQueryContext, _ string, args []driver.NamedValue) (context.Context, driver.Rows, error) {
	start := time.Now()
	defer mi.measure("stmt-query-context", "stmt-query-context", start)

	rows, err := conn.QueryContext(ctx, args)
	return ctx, rows, err
}

func (mi *metricInterceptor) StmtClose(ctx context.Context, conn driver.Stmt) error {
	start := time.Now()
	defer mi.measure("stmt-close", "stmt-close", start)
	return conn.Close()
}

// Tx interceptors
func (mi *metricInterceptor) TxCommit(ctx context.Context, conn driver.Tx) error {
	start := time.Now()
	defer mi.measure("tx-commit", "tx-commit", start)
	return conn.Commit()
}

func (mi *metricInterceptor) TxRollback(ctx context.Context, conn driver.Tx) error {
	start := time.Now()
	defer mi.measure("tx-rollback", "tx-rollback", start)
	return conn.Rollback()
}

func (mi *metricInterceptor) measure(op, method string, start time.Time) {
	labels := prometheus.Labels{
		"op": op,
	}
	pgOpTotal.With(labels).Inc()

	since := float64(time.Since(start).Milliseconds())
	labels = prometheus.Labels{
		"op":     op,
		"method": method,
	}
	pgOpLatency.With(labels).Observe(since)
}
