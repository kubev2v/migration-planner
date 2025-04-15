package log

import (
	"fmt"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func InitLog(lvl zap.AtomicLevel) *zap.Logger {
	const sampleInitial = 1 //first log message at each level will always be logged.
	const sampleThereafter = 5 //after the initial log, only every 5th subsequent log message at each level will be logged.

	cfg := zap.NewProductionConfig()
	cfg.Level = lvl
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg.Encoding = "json"

	cfg.Sampling = &zap.SamplingConfig{
		Initial:    sampleInitial,
		Thereafter: sampleThereafter,
	}

	logger, err := cfg.Build(
		zap.AddCaller(),
		zap.AddStacktrace(zap.DPanicLevel),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize zap logger: %v\n", err)
		panic(err)
	}

	return logger
}
