package utils

import (
	"os"
	"path/filepath"

	"github.com/kairos-io/immucore/internal/constants"
	"github.com/kairos-io/kairos-sdk/logger"
)

var Log *logger.KairosLog
var logFile *os.File

func CloseLogFiles() {
	logFile.Close()
}

func SetLogger() error {
	var err error
	err = os.MkdirAll(constants.LogDir, os.ModeDir|os.ModePerm)
	if err != nil {
		return err
	}

	Log, err = logger.NewKairosMultiLog(filepath.Join(constants.LogDir, "immucore.log"), logger.WithDebugFunction(DebugFunctionForLog))
	return err
}
