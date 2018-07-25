package logger

import (
	"fmt"
	"os"

	"github.com/coreos/etcd/raft"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

//Init returd new Zap logger
func Init(dev bool) (*zap.Logger, error) {

	var config zap.Config

	if !dev {
		config = productionConfig()
	} else {
		config = developmentConfig()
	}

	logger, err := config.Build(
		zap.Fields(
			zap.Int("pid", os.Getpid()),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("Error build zap logger [%s]", err)
	}
	return logger, nil
}

func developmentConfig() zap.Config {

	return zap.Config{
		Level:       zap.NewAtomicLevelAt(zap.DebugLevel),
		Development: true,
		Encoding:    "console",
		EncoderConfig: zapcore.EncoderConfig{
			// Keys can be anything except the empty string.
			TimeKey:        "T",
			LevelKey:       "L",
			NameKey:        "N",
			CallerKey:      "C",
			MessageKey:     "M",
			StacktraceKey:  "S",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    zapcore.CapitalColorLevelEncoder, // Enable color uotput
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeDuration: zapcore.StringDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		},
		OutputPaths:      []string{"stderr"},
		ErrorOutputPaths: []string{"stderr"},
	}
}

func productionConfig() zap.Config {
	return zap.Config{
		Level:       zap.NewAtomicLevelAt(zap.InfoLevel),
		Development: false,
		Sampling: &zap.SamplingConfig{
			Initial:    100,
			Thereafter: 100,
		},
		Encoding: "json",
		EncoderConfig: zapcore.EncoderConfig{
			TimeKey:        "ts",
			LevelKey:       "level",
			NameKey:        "logger",
			CallerKey:      "caller",
			MessageKey:     "msg",
			StacktraceKey:  "stacktrace",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    zapcore.LowercaseLevelEncoder,
			EncodeTime:     zapcore.EpochTimeEncoder,
			EncodeDuration: zapcore.SecondsDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		},
		OutputPaths:      []string{"stderr"},
		ErrorOutputPaths: []string{"stderr"},
	}
}

func NewZapLoggerRaft(logger *zap.SugaredLogger) raft.Logger {
	return &ZapLoggerRaft{
		loger: logger,
	}
}

type ZapLoggerRaft struct {
	loger *zap.SugaredLogger
}

func (l *ZapLoggerRaft) Debug(v ...interface{}) {
	l.loger.Debug(v...)
}

func (l *ZapLoggerRaft) Debugf(format string, v ...interface{}) {
	l.loger.Debugf(format, v...)
}

func (l *ZapLoggerRaft) Error(v ...interface{}) {
	l.loger.Error(v...)
}

func (l *ZapLoggerRaft) Errorf(format string, v ...interface{}) {
	l.loger.Errorf(format, v...)
}

func (l *ZapLoggerRaft) Info(v ...interface{}) {
	l.loger.Info(v...)
}

func (l *ZapLoggerRaft) Infof(format string, v ...interface{}) {
	l.loger.Infof(format, v...)
}

func (l *ZapLoggerRaft) Warning(v ...interface{}) {
	l.loger.Warn(v...)
}

func (l *ZapLoggerRaft) Warningf(format string, v ...interface{}) {
	l.loger.Warnf(format, v...)
}

func (l *ZapLoggerRaft) Fatal(v ...interface{}) {
	l.loger.Fatal(v...)
}

func (l *ZapLoggerRaft) Fatalf(format string, v ...interface{}) {
	l.loger.Fatalf(format, v...)
}

func (l *ZapLoggerRaft) Panic(v ...interface{}) {
	l.loger.Panic(v...)
}

func (l *ZapLoggerRaft) Panicf(format string, v ...interface{}) {
	l.loger.Panicf(format, v...)
}
