package utils

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/kairos-io/immucore/internal/constants"
	"github.com/rs/zerolog"
)

var Log zerolog.Logger
var logFile *os.File

func CloseLogFiles() {
	logFile.Close()
}

func SetLogger() {
	var loggers []io.Writer
	_ = os.MkdirAll(constants.LogDir, os.ModeDir|os.ModePerm)
	logFile, err := os.Create(filepath.Join(constants.LogDir, "immucore.log"))
	if err == nil {
		loggers = append(loggers, zerolog.ConsoleWriter{Out: logFile})
	}

	loggers = append(loggers, zerolog.ConsoleWriter{Out: os.Stderr})

	multi := zerolog.MultiLevelWriter(loggers...)
	Log = zerolog.New(multi).With().Logger().Level(zerolog.InfoLevel)
	// Set debug logger
	debug := len(ReadCMDLineArg("rd.immucore.debug")) > 0
	debugFromEnv := os.Getenv("IMMUCORE_DEBUG") != ""
	if debug || debugFromEnv {
		Log = zerolog.New(multi).With().Logger().Level(zerolog.InfoLevel)
	}
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
