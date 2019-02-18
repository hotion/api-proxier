package main

import (
	"flag"
	"net/http"
	"time"
	"log"
	
	"github.com/jademperor/api-proxier/internal/plugin"
	"github.com/jademperor/api-proxier/internal/plugin/cache/presistence"
	"github.com/jademperor/api-proxier/internal/plugin/cache"
	"github.com/jademperor/api-proxier/internal/plugin/rbac"
	"github.com/jademperor/api-proxier/internal/plugin/ratelimit"
	"github.com/jademperor/api-proxier/internal/plugin/httplog"
	"github.com/jademperor/api-proxier/internal/proxy"
	"github.com/jademperor/api-proxier/internal/logger"
	
	// "github.com/jademperor/common/pkg/etcd"
)

var (
	addr = flag.String("addr", ":9000", "http server listen on")
	logpath = flag.String("logpath", "./logs", "log files folder")
	etcdAddr = flag.String("etcd-addr", "etcd://127.0.0.1:2379", "addr of etcd store")
)

// TIMEOUT string
const TIMEOUT = "timeout"

// Engine ...
type Engine struct {
	allPlugins   []plugin.Plugin // all register plugins
	numAllPlugin int             // num of plugin
	addr         string          // gate addr
	proxier *proxy.Proxier // proxier
}

func (e *Engine) use(plgs ...plugin.Plugin) {
	e.allPlugins = append(e.allPlugins, plgs...)
	e.numAllPlugin += len(plgs)
}

func (e *Engine) init(addr string) {
	e.numAllPlugin = len(e.allPlugins)
}

func (e *Engine) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	ctx := plugin.NewContext(w, req, e.allPlugins)
	ctx.Next()

	if !ctx.Aborted() {
		e.proxier.Handle(ctx)
	}
	return
}

// Run ...
func (e *Engine) Run(addr string) error {
	e.init(addr)
	logger.Logger.WithFields(map[string]interface{}{
		"numPlugins": e.numAllPlugin,
		"addr":       addr,
	}).Info("start listening")

	handler := http.TimeoutHandler(e, 5*time.Second, TIMEOUT)
	return http.ListenAndServe(addr, handler)
}


func main() {
	flag.Parse()

	logger.InitLogger(*logpath)

	plgHTTPLogger := httplog.New(logger.Logger)
	plgCache := cache.New(presistence.NewInMemoryStore(), nil)
	plgTokenBucket := ratelimit.New(10, 1)
	plgRBAC := rbac.New("user_id", nil, nil)

	eng := &Engine{
		proxier: proxy.New(nil, nil, nil),	
	}

	// install plugins
	eng.use(plgHTTPLogger, plgTokenBucket, plgRBAC, plgCache)
	
	if err := eng.Run(*addr); err != nil {
		log.Fatal(err)
	}
}
