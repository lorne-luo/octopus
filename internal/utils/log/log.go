package log

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var Logger *zap.SugaredLogger
var atomicLevel = zap.NewAtomicLevelAt(zap.InfoLevel)
var consoleEncoder = zapcore.EncoderConfig{
	TimeKey:       "time",
	LevelKey:      "level",
	MessageKey:    "msg",
	CallerKey:     "caller",
	StacktraceKey: "stacktrace",
	EncodeLevel:   zapcore.CapitalLevelEncoder,
	EncodeTime:    zapcore.RFC3339TimeEncoder,
	EncodeCaller:  zapcore.ShortCallerEncoder,
}

func init() {
	SetupLogger("info", "")
}

func SetupLogger(level string, logPath string) {
	SetLevel(level)

	var cores []zapcore.Core

	// Console Core
	consoleCore := zapcore.NewCore(
		zapcore.NewConsoleEncoder(consoleEncoder),
		zapcore.AddSync(os.Stdout),
		atomicLevel,
	)
	cores = append(cores, consoleCore)

	// File Core
	if logPath != "" {
		w := zapcore.AddSync(&lumberjack.Logger{
			Filename:   logPath,
			MaxSize:    100, // megabytes
			MaxBackups: 3,
			MaxAge:     28, // days
		})
		fileCore := zapcore.NewCore(
			zapcore.NewConsoleEncoder(consoleEncoder),
			w,
			atomicLevel,
		)
		cores = append(cores, fileCore)
	}

	core := zapcore.NewTee(cores...)

	opts := []zap.Option{
		zap.AddCaller(),
		zap.AddCallerSkip(1),
		zap.AddStacktrace(zap.ErrorLevel),
	}
	Logger = zap.New(core, opts...).Sugar()
}

func SetLevel(level string) {
	var lvl zapcore.Level
	err := lvl.UnmarshalText([]byte(level))
	if err != nil {
		return
	}
	atomicLevel.SetLevel(lvl)
}

func Infof(template string, args ...interface{}) {
	Logger.Infof(template, args...)
}

func Errorf(template string, args ...interface{}) {
	Logger.Errorf(template, args...)
}

func Warnf(template string, args ...interface{}) {
	Logger.Warnf(template, args...)
}

func Debugf(template string, args ...interface{}) {
	Logger.Debugf(template, args...)
}
