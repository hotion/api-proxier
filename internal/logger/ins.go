// Package logger define output to std or file
package logger

import (
	pkglogger "github.com/jademperor/common/pkg/logger"
)

var (
	// Logger is an internal logger entity
	Logger *pkglogger.Entity
)

// Init call server-common to
func Init(logPath string, debug bool) (err error) {
	var (
		filename = "api-proxier.log"
		lv       = "info"
	)
	// open debug
	if debug {
		lv = "debug"
	}

	// Logger, err = pkglogger.NewJSONLogger(logPath, "api-proxier.log", "debug")
	Logger, err = pkglogger.NewTextLogger(logPath, filename, lv)
	return err
}
