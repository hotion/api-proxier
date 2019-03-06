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

type plgInfo struct {
	Name    string
	SoPath  string
	CfgData []byte
}

// InstallExtension get Plugin from ".so" file and init it
func InstallExtension(plgFlag string) (Plugin, error) {
	plgInfo, err := parseExtensionFlag(plgFlag)
	if err != nil {
		return nil, err
	}

	var (
		p       *osplugin.Plugin
		newFunc osplugin.Symbol
	)

	// load plugin.so and find the `New` Symbol of `func(cfgData []byte) Plugin`
	if p, err = osplugin.Open(plgInfo.SoPath); err != nil {
		return nil, err
	}
	if newFunc, err = p.Lookup("New"); err != nil {
		return nil, err
	}

	// call newFunc to generate the plugin.Plugin
	plg := newFunc.(func(cfgData []byte) Plugin)(plgInfo.CfgData)

	logger.Logger.Infof("extension [%s] parsed done", plgInfo.Name)
	return plg, nil
}

func parseExtensionFlag(plgFlag string) (*plgInfo, error) {
	pfs := strings.Split(plgFlag, ":")
	if len(pfs) < 2 || len(pfs) > 3 {
		return nil, errInvalidFlag
	}
	var (
		data []byte
		err  error
	)

	// with config file specified, so load it
	if len(pfs) == 3 {
		if data, err = ioutil.ReadFile(pfs[2]); err != nil {
			return nil, err
		}
	}

	return &plgInfo{
		Name:    pfs[0],
		SoPath:  pfs[1],
		CfgData: data,
	}, nil
}
