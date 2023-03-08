package utils

import (
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
	Log = zerolog.New(multi).With().Logger()
	Log.WithLevel(zerolog.InfoLevel)

	// Set debug logger
	debug := len(ReadCMDLineArg("rd.immucore.debug")) > 0
	debugFromEnv := os.Getenv("IMMUCORE_DEBUG") != ""
	if debug || debugFromEnv {
		Log = zerolog.New(multi).With().Caller().Logger()
		Log.WithLevel(zerolog.DebugLevel)
	}
}
