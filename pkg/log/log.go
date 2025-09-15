package log

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	gormlogger "gorm.io/gorm/logger"

	"github.com/kubev2v/migration-planner/pkg/requestid"
)

func InitLog(lvl zap.AtomicLevel) *zap.Logger {
	cfg := zap.NewProductionConfig()
	cfg.Level = lvl
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg.Encoding = "json"

	logger, err := cfg.Build(
		zap.AddStacktrace(zap.DPanicLevel),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize zap logger: %v\n", err)
		panic(err)
	}

	return logger
}

// StructuredLogger provides structured logging for business services at specified level
type StructuredLogger struct {
	logger  *zap.Logger
	service string
	level   zapcore.Level
}

// NewStructuredLogger creates a new structured logger for a specific service at the given level
func NewStructuredLogger(service string, level zapcore.Level) *StructuredLogger {
	return &StructuredLogger{
		logger:  zap.L().Named(service),
		service: service,
		level:   level,
	}
}

// NewDebugLogger creates a new debug-level structured logger for a specific service
func NewDebugLogger(service string) *StructuredLogger {
	return NewStructuredLogger(service, zapcore.DebugLevel)
}

// NewInfoLogger creates a new info-level structured logger for a specific service
func NewInfoLogger(service string) *StructuredLogger {
	return NewStructuredLogger(service, zapcore.InfoLevel)
}

// NewWarnLogger creates a new warn-level structured logger for a specific service
func NewWarnLogger(service string) *StructuredLogger {
	return NewStructuredLogger(service, zapcore.WarnLevel)
}

// DebugLogger is an alias for backward compatibility
type DebugLogger = StructuredLogger

// getLogFunc returns the appropriate logging function based on the configured level
func (l *StructuredLogger) getLogFunc() func(msg string, fields ...zap.Field) {
	switch l.level {
	case zapcore.DebugLevel:
		return l.logger.Debug
	case zapcore.InfoLevel:
		return l.logger.Info
	case zapcore.WarnLevel:
		return l.logger.Warn
	case zapcore.ErrorLevel:
		return l.logger.Error
	default:
		return l.logger.Debug
	}
}

// WithContext returns a new StructuredLogger with request context
func (l *StructuredLogger) WithContext(ctx context.Context) *StructuredLogger {
	// Extract request ID if available
	if requestID := requestid.FromContext(ctx); requestID != "" {
		return &StructuredLogger{
			logger:  l.logger.With(zap.String("request_id", requestID)),
			service: l.service,
			level:   l.level,
		}
	}
	return l
}

// Operation begins operation tracing and returns a builder
func (l *StructuredLogger) Operation(operation string) *OperationBuilder {
	return &OperationBuilder{
		operation: operation,
		fields:    make([]zap.Field, 0),
		logger:    l,
	}
}

// OperationBuilder builds operation parameters fluently
type OperationBuilder struct {
	operation string
	fields    []zap.Field
	logger    *StructuredLogger
}

// WithParam adds a generic parameter
func (b *OperationBuilder) WithParam(key string, value any) *OperationBuilder {
	b.fields = append(b.fields, zap.Any(key, value))
	return b
}

// WithString adds a string parameter
func (b *OperationBuilder) WithString(key, value string) *OperationBuilder {
	b.fields = append(b.fields, zap.String(key, value))
	return b
}

// WithInt adds an int parameter
func (b *OperationBuilder) WithInt(key string, value int) *OperationBuilder {
	b.fields = append(b.fields, zap.Int(key, value))
	return b
}

// WithBool adds a bool parameter
func (b *OperationBuilder) WithBool(key string, value bool) *OperationBuilder {
	b.fields = append(b.fields, zap.Bool(key, value))
	return b
}

// WithUUID adds a UUID parameter
func (b *OperationBuilder) WithUUID(key string, value uuid.UUID) *OperationBuilder {
	b.fields = append(b.fields, zap.String(key, value.String()))
	return b
}

// WithUUIDPtr adds a UUID pointer parameter (nil-safe)
func (b *OperationBuilder) WithUUIDPtr(key string, value *uuid.UUID) *OperationBuilder {
	if value != nil {
		b.fields = append(b.fields, zap.String(key, value.String()))
	} else {
		b.fields = append(b.fields, zap.Any(key, nil))
	}
	return b
}

// WithStringPtr adds a string pointer parameter (nil-safe)
func (b *OperationBuilder) WithStringPtr(key string, value *string) *OperationBuilder {
	if value != nil {
		b.fields = append(b.fields, zap.String(key, *value))
	} else {
		b.fields = append(b.fields, zap.Any(key, nil))
	}
	return b
}

// WithIntPtr adds an int pointer parameter (nil-safe)
func (b *OperationBuilder) WithIntPtr(key string, value *int) *OperationBuilder {
	if value != nil {
		b.fields = append(b.fields, zap.Int(key, *value))
	} else {
		b.fields = append(b.fields, zap.Any(key, nil))
	}
	return b
}

func (b *OperationBuilder) WithRequestBody(key string, value any) *OperationBuilder {
	if value != nil {
		b.fields = append(b.fields, zap.Any(key, value))
	}
	return b
}

// Build creates and starts the operation tracer
func (b *OperationBuilder) Build() *OperationTracer {
	logFunc := b.logger.getLogFunc()
	fields := append([]zap.Field{}, b.fields...)
	logFunc("operation started", fields...)

	return &OperationTracer{
		StructuredLogger: b.logger,
		operation:        b.operation,
		fields:           b.fields,
	}
}

// OperationTracer tracks the progress of a business operation
type OperationTracer struct {
	*StructuredLogger
	operation string
	fields    []zap.Field
}

// Step creates a step builder
func (ot *OperationTracer) Step(step string) *StepBuilder {
	return &StepBuilder{
		tracer: ot,
		step:   step,
		fields: make([]zap.Field, 0),
	}
}

// Success creates a result builder
func (ot *OperationTracer) Success() *ResultBuilder {
	return &ResultBuilder{
		tracer:  ot,
		fields:  make([]zap.Field, 0),
		isError: false,
	}
}

// Error creates an error result builder that logs at error level
func (ot *OperationTracer) Error(err error) *ResultBuilder {
	return &ResultBuilder{
		tracer:  ot,
		fields:  []zap.Field{zap.String("error", err.Error())},
		isError: true,
	}
}

// StepBuilder builds step data fluently
type StepBuilder struct {
	tracer *OperationTracer
	step   string
	fields []zap.Field
}

// WithParam adds a generic parameter to the step
func (b *StepBuilder) WithParam(key string, value any) *StepBuilder {
	b.fields = append(b.fields, zap.Any(key, value))
	return b
}

// WithString adds a string parameter to the step
func (b *StepBuilder) WithString(key, value string) *StepBuilder {
	b.fields = append(b.fields, zap.String(key, value))
	return b
}

// WithInt adds an int parameter to the step
func (b *StepBuilder) WithInt(key string, value int) *StepBuilder {
	b.fields = append(b.fields, zap.Int(key, value))
	return b
}

// WithBool adds a bool parameter to the step
func (b *StepBuilder) WithBool(key string, value bool) *StepBuilder {
	b.fields = append(b.fields, zap.Bool(key, value))
	return b
}

// WithUUID adds a UUID parameter to the step
func (b *StepBuilder) WithUUID(key string, value uuid.UUID) *StepBuilder {
	b.fields = append(b.fields, zap.String(key, value.String()))
	return b
}

// WithUUIDPtr adds a UUID pointer parameter to the step (nil-safe)
func (b *StepBuilder) WithUUIDPtr(key string, value *uuid.UUID) *StepBuilder {
	if value != nil {
		b.fields = append(b.fields, zap.String(key, value.String()))
	} else {
		b.fields = append(b.fields, zap.Any(key, nil))
	}
	return b
}

// WithStringPtr adds a string pointer parameter to the step (nil-safe)
func (b *StepBuilder) WithStringPtr(key string, value *string) *StepBuilder {
	if value != nil {
		b.fields = append(b.fields, zap.String(key, *value))
	} else {
		b.fields = append(b.fields, zap.Any(key, nil))
	}
	return b
}

// WithIntPtr adds an int pointer parameter to the step (nil-safe)
func (b *StepBuilder) WithIntPtr(key string, value *int) *StepBuilder {
	if value != nil {
		b.fields = append(b.fields, zap.Int(key, *value))
	} else {
		b.fields = append(b.fields, zap.Any(key, nil))
	}
	return b
}

// Log executes the step logging
func (b *StepBuilder) Log() {
	logFunc := b.tracer.getLogFunc()
	fields := []zap.Field{
		zap.String("operation", b.tracer.operation),
		zap.String("step", b.step),
	}
	fields = append(fields, b.fields...)
	logFunc("operation step", fields...)
}

// ResultBuilder builds result data fluently
type ResultBuilder struct {
	tracer  *OperationTracer
	fields  []zap.Field
	isError bool
}

// WithParam adds a generic parameter to the result
func (b *ResultBuilder) WithParam(key string, value any) *ResultBuilder {
	b.fields = append(b.fields, zap.Any(key, value))
	return b
}

// WithString adds a string parameter to the result
func (b *ResultBuilder) WithString(key, value string) *ResultBuilder {
	b.fields = append(b.fields, zap.String(key, value))
	return b
}

// WithInt adds an int parameter to the result
func (b *ResultBuilder) WithInt(key string, value int) *ResultBuilder {
	b.fields = append(b.fields, zap.Int(key, value))
	return b
}

// WithBool adds a bool parameter to the result
func (b *ResultBuilder) WithBool(key string, value bool) *ResultBuilder {
	b.fields = append(b.fields, zap.Bool(key, value))
	return b
}

// WithUUID adds a UUID parameter to the result
func (b *ResultBuilder) WithUUID(key string, value uuid.UUID) *ResultBuilder {
	b.fields = append(b.fields, zap.String(key, value.String()))
	return b
}

// WithUUIDPtr adds a UUID pointer parameter to the result (nil-safe)
func (b *ResultBuilder) WithUUIDPtr(key string, value *uuid.UUID) *ResultBuilder {
	if value != nil {
		b.fields = append(b.fields, zap.String(key, value.String()))
	} else {
		b.fields = append(b.fields, zap.Any(key, nil))
	}
	return b
}

// WithStringPtr adds a string pointer parameter to the result (nil-safe)
func (b *ResultBuilder) WithStringPtr(key string, value *string) *ResultBuilder {
	if value != nil {
		b.fields = append(b.fields, zap.String(key, *value))
	} else {
		b.fields = append(b.fields, zap.Any(key, nil))
	}
	return b
}

// WithIntPtr adds an int pointer parameter to the result (nil-safe)
func (b *ResultBuilder) WithIntPtr(key string, value *int) *ResultBuilder {
	if value != nil {
		b.fields = append(b.fields, zap.Int(key, *value))
	} else {
		b.fields = append(b.fields, zap.Any(key, nil))
	}
	return b
}

// WithError adds the error to the log (typically used with Failed())
func (b *ResultBuilder) WithError(err error) *ResultBuilder {
	b.fields = append(b.fields, zap.String("error", err.Error()))
	return b
}

// Log executes the result logging
func (b *ResultBuilder) Log() {
	fields := []zap.Field{
		zap.String("operation", b.tracer.operation),
	}
	fields = append(fields, b.fields...)

	if b.isError {
		b.tracer.logger.Error("operation failed", fields...)
	} else {
		logFunc := b.tracer.getLogFunc()
		logFunc("operation completed", fields...)
	}
}

// GormLogger implements the GORM logger interface using our structured logger
type GormLogger struct {
	logger   *StructuredLogger
	logLevel gormlogger.LogLevel
	config   gormlogger.Config
}

// NewGormLogger creates a new GORM logger that bridges to our structured logger
func NewGormLogger(service string, config gormlogger.Config) gormlogger.Interface {
	return &GormLogger{
		logger:   NewDebugLogger(service),
		logLevel: config.LogLevel,
		config:   config,
	}
}

// NewDefaultGormLogger creates a GORM logger with default configuration
func NewDefaultGormLogger(service string) gormlogger.Interface {
	return NewGormLogger(service, gormlogger.Config{
		SlowThreshold:             200 * time.Millisecond,
		LogLevel:                  gormlogger.Warn,
		IgnoreRecordNotFoundError: false,
		Colorful:                  false,
		ParameterizedQueries:      false,
	})
}

// LogMode implements gorm logger interface
func (g *GormLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	return &GormLogger{
		logger:   g.logger,
		logLevel: level,
		config:   g.config,
	}
}

// Info implements gorm logger interface
func (g *GormLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	if g.logLevel >= gormlogger.Info {
		contextLogger := g.logger.WithContext(ctx)
		contextLogger.Operation("gorm").
			WithString("level", "info").
			WithString("message", fmt.Sprintf(msg, data...)).
			Build().Success().Log()
	}
}

// Warn implements gorm logger interface
func (g *GormLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	if g.logLevel >= gormlogger.Warn {
		contextLogger := g.logger.WithContext(ctx)
		contextLogger.Operation("gorm").
			WithString("level", "warn").
			WithString("message", fmt.Sprintf(msg, data...)).
			Build().Success().Log()
	}
}

// Error implements gorm logger interface
func (g *GormLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	if g.logLevel >= gormlogger.Error {
		contextLogger := g.logger.WithContext(ctx)
		contextLogger.Operation("gorm").
			WithString("level", "error").
			WithString("message", fmt.Sprintf(msg, data...)).
			Build().Success().Log()
	}
}

// Trace implements gorm logger interface
func (g *GormLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if g.logLevel <= gormlogger.Silent {
		return
	}

	elapsed := time.Since(begin)
	contextLogger := g.logger.WithContext(ctx)

	switch {
	case err != nil && g.logLevel >= gormlogger.Error && !g.shouldIgnoreError(err):
		sql, rows := fc()
		contextLogger.Operation("gorm_query").
			WithString("sql", sql).
			WithInt("rows_affected", int(rows)).
			WithParam("duration_ms", float64(elapsed.Nanoseconds())/1e6).
			Build().Error(err).Log()

	case elapsed > g.config.SlowThreshold && g.config.SlowThreshold != 0 && g.logLevel >= gormlogger.Warn:
		sql, rows := fc()
		contextLogger.Operation("gorm_slow_query").
			WithString("sql", sql).
			WithInt("rows_affected", int(rows)).
			WithParam("duration_ms", float64(elapsed.Nanoseconds())/1e6).
			WithParam("threshold_ms", float64(g.config.SlowThreshold.Nanoseconds())/1e6).
			Build().Success().Log()

	case g.logLevel == gormlogger.Info:
		sql, rows := fc()
		contextLogger.Operation("gorm_query").
			WithString("sql", sql).
			WithInt("rows_affected", int(rows)).
			WithParam("duration_ms", float64(elapsed.Nanoseconds())/1e6).
			Build().Success().Log()
	}
}

// shouldIgnoreError checks if the error should be ignored based on configuration
func (g *GormLogger) shouldIgnoreError(err error) bool {
	return g.config.IgnoreRecordNotFoundError && err == gormlogger.ErrRecordNotFound
}
