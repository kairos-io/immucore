package utils

import (
	"github.com/rs/zerolog"
	"io"
	"os"
)

var Log zerolog.Logger
var logFile *os.File

func CloseLogFiles() {
	logFile.Close()
}

func SetLogger() {
	var loggers []io.Writer
	logFile, err := os.Create("/run/immucore.log")
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
