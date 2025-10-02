package logging

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/trace"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// contextKey is a private string type to prevent collisions in the context map.
type contextKey string

// loggerKey points to the value in the context where the logger is stored.
const loggerKey = contextKey("logger")

func NewLogger(level string, format string) *zap.SugaredLogger {
	if flag.Lookup("test.v") != nil {
		return zap.NewNop().Sugar()
	}

	loggerCfg := zap.NewProductionConfig()
	switch strings.ToLower(format) {
	case "zap-pretty":
		loggerCfg = zap.NewProductionConfig()
		loggerCfg.EncoderConfig.EncodeTime = timeEncoderTS()
		loggerCfg.EncoderConfig.TimeKey = "ts"
		loggerCfg.EncoderConfig.MessageKey = "msg"
	case "text":
		loggerCfg = zap.NewDevelopmentConfig()
	case "json":
		loggerCfg.EncoderConfig.MessageKey = "message"
		loggerCfg.EncoderConfig.EncodeTime = defaultTimeEncoder()
		loggerCfg.EncoderConfig.TimeKey = "timestamp"
		loggerCfg.EncoderConfig.EncodeDuration = zapcore.NanosDurationEncoder
		loggerCfg.EncoderConfig.StacktraceKey = "errorstack"
		loggerCfg.EncoderConfig.FunctionKey = "logger.method_name"
	default:
		panic(fmt.Sprintf("invalid log format. possible values: json, text, zap-pretty. %s given", format))
	}

	zapLevel, err := zap.ParseAtomicLevel(strings.ToLower(level))
	if err != nil {
		panic(fmt.Sprintf("invalid log level. possible values: debug, info, warn, error. %s given", level))
	}
	loggerCfg.Level = zapLevel

	logger, err := loggerCfg.Build()
	if err != nil {
		panic(err)
	}

	environment := os.Getenv("ENVIRONMENT")
	if environment != "" {
		logger = logger.With(zap.String("environment", environment))
	}

	return logger.Sugar()
}

// defaultTimeEncoder encodes the time as RFC3339 nano.
func defaultTimeEncoder() zapcore.TimeEncoder {
	return func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(t.Format(time.RFC3339Nano))
	}
}

// timeEncoderTS encodes the time as a Unix timestamp (seconds since the Unix epoch).
func timeEncoderTS() zapcore.TimeEncoder {
	return func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendInt64(t.Unix())
	}
}

func WithLogger(ctx context.Context, logger *zap.SugaredLogger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

func FromContext(ctx context.Context) *zap.SugaredLogger {
	if logger, ok := ctx.Value(loggerKey).(*zap.SugaredLogger); ok {
		spanCtx := trace.SpanContextFromContext(ctx)
		if spanCtx.HasTraceID() {
			traceID := spanCtx.TraceID().String()
			spanID := spanCtx.SpanID().String()
			logger = logger.With(zap.String("dd.trace_id", convertTraceID(traceID)), zap.String("dd.span_id", convertTraceID(spanID)), zap.String("otel_trace_id", spanCtx.TraceID().String()))
		}
		return logger
	}

	l := NewLogger("info", "json")
	l.Warn("logger not found in context, using default json logger")
	return l
}

func convertTraceID(id string) string {
	if len(id) < 16 {
		return ""
	}
	if len(id) > 16 {
		id = id[16:]
	}
	intValue, err := strconv.ParseUint(id, 16, 64)
	if err != nil {
		return ""
	}
	return strconv.FormatUint(intValue, 10)
}

func WithTestLogger(ctx context.Context) (context.Context, *observer.ObservedLogs) {
	observedZapCore, observedLogs := observer.New(zap.InfoLevel)
	observedLoggerSugared := zap.New(observedZapCore).Sugar()

	return WithLogger(ctx, observedLoggerSugared), observedLogs
}

func LogCommandExecution(ctx context.Context, commandName string, cmd interface{}, err error) {
	logger := FromContext(ctx).With(
		zap.Error(err),
		zap.String("command.name", commandName),
		zap.Any("command.data", cmd),
	).Desugar().WithOptions(zap.AddCallerSkip(1)).Sugar()

	if err == nil {
		logger.Info(commandName + " command succeed")
	} else {
		logger.Error(commandName + " command failed")
	}
}

func PanicHandler(logger *zap.SugaredLogger) {
	if r := recover(); r != nil {
		logger.Desugar().WithOptions(zap.AddCallerSkip(1)).Sugar().DPanicf("panic: %v", r)
	}
}

func NewLeveledLogger(logger *zap.SugaredLogger) *LeveledLogger {
	return &LeveledLogger{logger: logger.Desugar().WithOptions(zap.AddCallerSkip(1)).Sugar()}
}

// LeveledLogger interface implements the basic methods that a logger library needs.
type LeveledLogger struct {
	logger *zap.SugaredLogger
}

func (l *LeveledLogger) Error(msg string, keysAndVals ...interface{}) {
	l.logger.Errorw(msg, keysAndVals...)
}

func (l *LeveledLogger) Info(msg string, keysAndVals ...interface{}) {
	l.logger.Infow(msg, keysAndVals...)
}

func (l *LeveledLogger) Debug(msg string, keysAndVals ...interface{}) {
	l.logger.Debugw(msg, keysAndVals...)
}

func (l *LeveledLogger) Warn(msg string, keysAndVals ...interface{}) {
	l.logger.Warnw(msg, keysAndVals...)
}

func (l *LeveledLogger) Log(msg string) {
	l.logger.Info(msg)
}
