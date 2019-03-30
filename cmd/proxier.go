package main

import (
	"flag"
	"log"
	"os"

	"github.com/jademperor/api-proxier/internal/engine"
	"github.com/jademperor/api-proxier/internal/logger"
	"github.com/jademperor/common/pkg/utils"
)

var (
	addr      = flag.String("addr", ":9000", "http server listen on")
	logpath   = flag.String("logpath", "./logs", "log files folder")
	debug     = flag.Bool("debug", false, "open debug")
	plugins   utils.StringArray
	etcdAddrs utils.StringArray
)

func main() {
	flag.Var(&etcdAddrs, "etcd-addr", "addr of etcd store")
	flag.Var(&plugins, "plugin", "plugin extension format like: [pluginName:plugin.so:/path/to/config]")
	flag.Parse()

	// valid command line arguments
	if len(etcdAddrs) == 0 {
		log.Println("etcd-addr must be set one or more values")
		os.Exit(-1)
	}

	// init logger configuration
	logger.Init(*logpath, *debug)

	// new engine to run
	e, err := engine.New(etcdAddrs, plugins, *debug)
	if err != nil {
		log.Fatal(err)
	}

	// run the server and serve with http request
	if err := e.Run(*addr); err != nil {
		log.Fatal(err)
	}
}
