package utils

import (
	"github.com/rs/zerolog"
	"golang.org/x/sys/unix"
	"io"
	"os"
)

var Log zerolog.Logger

func SetLogger() {
	var loggers []io.Writer
	devKmsg, err := os.OpenFile("/dev/kmsg", unix.O_APPEND, 0o600)
	if err == nil {
		loggers = append(loggers, zerolog.ConsoleWriter{Out: devKmsg})
	}
	logFile, err := os.Create("/run/immucore.log")
	if err == nil {
		loggers = append(loggers, zerolog.ConsoleWriter{Out: logFile})
	}

	// No loggers? Then stdout ¯\_(ツ)_/¯
	if len(loggers) == 0 {
		loggers = append(loggers, zerolog.ConsoleWriter{Out: os.Stdout})
	}
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