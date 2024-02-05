package logging

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hexiaodai/virtnet/pkg/env"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	LogLevelInfo  LogLevel = "info"
	LogLevelDebug LogLevel = "debug"
	LogLevelError LogLevel = "error"
	LogLevelWarn  LogLevel = "warn"
)

const (
	DefaultIPAMPluginLogFilePath        = "/var/log/virtnet/virtnet-ipam.log"
	DefaultCoordinatorPluginLogFilePath = "/var/log/virtnet/virtnet-coordinator.log"
	DefaultAgentLogFilePath             = "/var/log/virtnet/virtnet-agent.log"
	DefaultCtrlLogFilePath              = "/var/log/virtnet/virtnet-ctrl.log"

	// MaxSize    = 100 // MB
	DefaultLogFileMaxSize int = 100
	// MaxAge     = 30 // days (no limit)
	DefaultLogFileMaxAge = 30
	// MaxBackups = 10 // no limit
	DefaultLogFileMaxBackups = 10
)

const (
	OUTPUT_STDOUT LogMode = iota
	OUTPUT_STDERR
	OUTPUT_FILE
)

const (
	FORMAT_CONSOLE LogFormat = iota
	FORMAT_JSON
)

// LogMode is a type help you choose the log output mode, which supports "stderr","stdout","file".
type LogMode int

// LogLevel is a type help you choose the log level.
type LogLevel string

type LogFormat int

type Logging struct {
	// Level is the logging level. If unspecified, defaults to "info".
	// LogLevel options: debug/info/error/warn.
	Level LogLevel
	// LogMode options: OUTPUT_FILE/OUTPUT_STDERR/OUTPUT_STDOUT
	OutputMode map[LogMode]struct{}
	// Log Output Path
	OutputFilePath string
	// Log Format
	Format LogFormat
}

// A Sugar wraps the base Logger functionality in a slower, but less
// verbose, API. Any Logger can be converted to a SugaredLogger with its Sugar
// method.
//
// Unlike the Logger, the SugaredLogger doesn't insist on structured logging.
// For each log level, it exposes four methods:
//
//   - methods named after the log level for log.Print-style logging
//   - methods ending in "w" for loosely-typed structured logging
//   - methods ending in "f" for log.Printf-style logging
//   - methods ending in "ln" for log.Println-style logging
//
// For example, the methods for InfoLevel are:
//
//	Info(...any)           Print-style logging
//	Infow(...any)          Structured logging (read as "info with")
//	Infof(string, ...any)  Printf-style logging
//	Infoln(...any)         Println-style logging
type Logger struct {
	*zap.SugaredLogger
}

func NewLogger(log *Logging) Logger {
	if log.OutputMode == nil || len(log.OutputMode) == 0 {
		log.OutputMode = map[LogMode]struct{}{
			OUTPUT_STDOUT: {},
		}
	}
	logger := initZapLogger(log)
	return Logger{
		logger.Sugar(),
	}
}

func DefaultLogger() Logger {
	return NewLogger(&Logging{
		Level: LogLevel(env.Lookup("LOG_LEVEL", string(LogLevelInfo))),
	})
}

func LoggerFromOutputFile(level LogLevel, filePath string) Logger {
	return NewLogger(&Logging{
		Level: level,
		OutputMode: map[LogMode]struct{}{
			OUTPUT_FILE: {},
		},
		OutputFilePath: filePath,
		Format:         FORMAT_JSON,
	})
}

func initZapLogger(log *Logging) *zap.Logger {
	parseLevel, err := zapcore.ParseLevel(string(log.Level))
	if err != nil {
		panic(err)
	}

	var fileLoggerConf lumberjack.Logger
	if _, ok := log.OutputMode[OUTPUT_FILE]; ok {
		fileLoggerConf = lumberjack.Logger{
			Filename:   log.OutputFilePath,
			MaxSize:    DefaultLogFileMaxSize,
			MaxAge:     DefaultLogFileMaxAge,
			MaxBackups: DefaultLogFileMaxBackups,
		}
		err := os.MkdirAll(filepath.Dir(log.OutputFilePath), 0755)
		if nil != err {
			panic(fmt.Sprintf("failed to create path for CNI log file: %v", filepath.Dir(log.OutputFilePath)))
		}
	}

	var wss []zapcore.WriteSyncer
	for outputMode := range log.OutputMode {
		switch outputMode {
		case OUTPUT_FILE:
			wss = append(wss, zapcore.AddSync(&fileLoggerConf))
		case OUTPUT_STDOUT:
			wss = append(wss, zapcore.AddSync(os.Stdout))
		case OUTPUT_STDERR:
			wss = append(wss, zapcore.AddSync(os.Stderr))
		}
	}
	if len(wss) == 0 {
		wss = append(wss, zapcore.AddSync(os.Stdout))
	}

	// set zap encoder configuration
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeCaller = zapcore.ShortCallerEncoder
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder

	var encoder zapcore.Encoder
	switch log.Format {
	case FORMAT_JSON:
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	case FORMAT_CONSOLE:
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	default:
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	}

	core := zapcore.NewCore(
		// zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig()),
		encoder,
		zapcore.NewMultiWriteSyncer(wss...),
		zap.NewAtomicLevelAt(parseLevel))

	return zap.New(core, zap.AddCaller())
}

// loggerKey is how we find Loggers in a context.Context.
type loggerKey struct{}

// FromContext returns a logger with predefined values from a context.Context.
func FromContext(ctx context.Context) *zap.SugaredLogger {
	log := DefaultLogger().SugaredLogger
	if ctx != nil {
		if logger, ok := ctx.Value(loggerKey{}).(*zap.SugaredLogger); ok {
			log = logger
		}
	}

	return log
}

// IntoContext takes a context and sets the logger as one of its values.
// Use FromContext function to retrieve the logger.
func IntoContext(ctx context.Context, logger *zap.SugaredLogger) context.Context {
	return context.WithValue(ctx, loggerKey{}, logger)
}
