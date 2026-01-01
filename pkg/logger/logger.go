package logger

import (
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	Log   *zap.Logger
	Sugar *zap.SugaredLogger
)

func init() {
	// Create logs directory if it doesn't exist
	if err := os.MkdirAll("logs", 0755); err != nil {
		panic(err)
	}

	// Open log file
	file, err := os.OpenFile("logs/p2p-transfer.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		panic(err)
	}

	// Custom encoder config for file output
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(t.Format("2006/01/02 15:04:05"))
	}
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	encoderConfig.EncodeCaller = zapcore.ShortCallerEncoder

	// Use ConsoleEncoder for human-readable output in file
	// If JSON is preferred for parsing, we could switch to NewJSONEncoder
	fileEncoder := zapcore.NewConsoleEncoder(encoderConfig)

	level := zapcore.InfoLevel
	levelStr := strings.TrimSpace(os.Getenv("P2P_LOG_LEVEL"))
	if levelStr == "" {
		levelStr = strings.TrimSpace(os.Getenv("LOG_LEVEL"))
	}
	if levelStr != "" {
		_ = level.UnmarshalText([]byte(strings.ToLower(levelStr)))
	}

	core := zapcore.NewCore(
		fileEncoder,
		zapcore.AddSync(file),
		level,
	)

	// AddCaller ensures the log includes filename and line number
	Log = zap.New(core, zap.AddCaller())
	Sugar = Log.Sugar()
}
