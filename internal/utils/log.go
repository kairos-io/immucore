package utils

import (
	"fmt"
	"github.com/kairos-io/kairos-sdk/types"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/kairos-io/immucore/internal/constants"
	"github.com/rs/zerolog"
)

var Log zerolog.Logger
var logFile *os.File

// KLog is the generic KairosLogger that we pass to kcrypt calls
var KLog types.KairosLogger

func CloseLogFiles() {
	logFile.Close()
}

func SetLogger() {
	var loggers []io.Writer
	var level zerolog.Level

	_ = os.MkdirAll(constants.LogDir, os.ModeDir|os.ModePerm)
	logFile, err := os.Create(filepath.Join(constants.LogDir, "immucore.log"))
	if err == nil {
		loggers = append(loggers, zerolog.ConsoleWriter{Out: logFile, TimeFormat: time.RFC3339, NoColor: true})
	}

	loggers = append(loggers, zerolog.NewConsoleWriter(func(w *zerolog.ConsoleWriter) {
		w.TimeFormat = time.RFC3339
	}))

	multi := zerolog.MultiLevelWriter(loggers...)
	level = zerolog.InfoLevel
	// Set debug level
	debug := len(ReadCMDLineArg("rd.immucore.debug")) > 0
	debugFromEnv := os.Getenv("IMMUCORE_DEBUG") != ""
	if debug || debugFromEnv {
		level = zerolog.DebugLevel
	}
	Log = zerolog.New(multi).With().Timestamp().Logger().Level(level)
	KLog = types.NewNullLogger()
	KLog.Logger = Log
}

// MiddleLog implements the bridge between zerolog and the logger.Interface that yip needs.
type MiddleLog struct {
	zerolog.Logger
}

func (m MiddleLog) Infof(tpl string, args ...interface{}) {
	m.Logger.Info().Msg(fmt.Sprintf(tpl, args...))
}
func (m MiddleLog) Info(args ...interface{}) {
	m.Logger.Info().Msg(fmt.Sprint(args...))
}
func (m MiddleLog) Warnf(tpl string, args ...interface{}) {
	m.Logger.Warn().Msg(fmt.Sprintf(tpl, args...))
}
func (m MiddleLog) Warn(args ...interface{}) {
	m.Logger.Warn().Msg(fmt.Sprint(args...))
}
func (m MiddleLog) Debugf(tpl string, args ...interface{}) {
	m.Logger.Debug().Msg(fmt.Sprintf(tpl, args...))
}
func (m MiddleLog) Debug(args ...interface{}) {
	m.Logger.Debug().Msg(fmt.Sprint(args...))
}
func (m MiddleLog) Errorf(tpl string, args ...interface{}) {
	m.Logger.Error().Msg(fmt.Sprintf(tpl, args...))
}
func (m MiddleLog) Error(args ...interface{}) {
	m.Logger.Error().Msg(fmt.Sprint(args...))
}
func (m MiddleLog) Fatalf(tpl string, args ...interface{}) {
	m.Logger.Fatal().Msg(fmt.Sprintf(tpl, args...))
}
func (m MiddleLog) Fatal(args ...interface{}) {
	m.Logger.Fatal().Msg(fmt.Sprint(args...))
}
func (m MiddleLog) Panicf(tpl string, args ...interface{}) {
	m.Logger.Panic().Msg(fmt.Sprintf(tpl, args...))
}
func (m MiddleLog) Panic(args ...interface{}) {
	m.Logger.Panic().Msg(fmt.Sprint(args...))
}
func (m MiddleLog) Tracef(tpl string, args ...interface{}) {
	m.Logger.Trace().Msg(fmt.Sprintf(tpl, args...))
}
func (m MiddleLog) Trace(args ...interface{}) {
	m.Logger.Trace().Msg(fmt.Sprint(args...))
}
