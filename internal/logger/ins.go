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
func Init(logPath string) (err error) {
	// Logger, err = pkglogger.NewJSONLogger(logPath, "api-proxier.log", "debug")
	Logger, err = pkglogger.NewTextLogger(logPath, "api-proxier.log", "debug")
	return err
}
