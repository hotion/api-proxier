package plugin

import (
	"errors"
	"io/ioutil"
	"log"
	osplugin "plugin"
	"runtime/debug"
	"strings"

	"github.com/jademperor/api-proxier/internal/logger"
)

var (
	errInvalidFlag = errors.New("invalid plugin flag: [pluginName:plugin.so:/path/to/config.json]")
)

// Recover func to get panic detail
func Recover(plgName string) {
	if v := recover(); v != nil {
		err, ok := v.(error)
		if !ok {
			log.Printf("plugin.%s panic: %s\n", plgName, debug.Stack())
			return
		}
		log.Printf("plugin.%s panic error: %v\n stack %s", plgName, err, debug.Stack())
	}
}

// ParseExtension get Plugin from ".so" file and init it
func ParseExtension(plgFlag string) (Plugin, error) {
	pfs, err := validExtensionFlag(plgFlag)
	if err != nil {
		return nil, err
	}
	var (
		plgName    = pfs[0]
		plgSoPath  = pfs[1]
		plgCfgData []byte
	)
	if len(pfs) == 3 {
		// hasConfig and load
		if plgCfgData, err = ioutil.ReadFile(pfs[2]); err != nil {
			return nil, err
		}
	}

	var (
		p       *osplugin.Plugin
		newFunc osplugin.Symbol
	)
	// load plugin.so and find the `New` Symbol of `func(cfgData []byte) Plugin`
	if p, err = osplugin.Open(plgSoPath); err != nil {
		return nil, err
	}
	if newFunc, err = p.Lookup("New"); err != nil {
		return nil, err
	}

	// call newFunc to generate the plugin.Plugin
	plg := newFunc.(func(cfgData []byte) Plugin)(plgCfgData)

	logger.Logger.Infof("extension [%s] parsed done", plgName)
	return plg, nil
}

// validExtensionFlag ...
func validExtensionFlag(plgFlag string) (pfs []string, err error) {
	pfs = strings.Split(plgFlag, ":")
	if len(pfs) < 2 || len(pfs) > 3 {
		err = errInvalidFlag
		return
	}
	return
}
