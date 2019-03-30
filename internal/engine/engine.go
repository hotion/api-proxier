package engine

import (
	// "context"
	// "encoding/json"
	"net/http"
	"net/http/pprof"
	"strings"
	"time"

	"github.com/jademperor/api-proxier/internal/logger"
	"github.com/jademperor/api-proxier/internal/proxy"
	"github.com/jademperor/api-proxier/internal/stdplugin/httplog"
	"github.com/jademperor/api-proxier/internal/stdplugin/ratelimit"
	"github.com/jademperor/api-proxier/plugin"
	"github.com/jademperor/common/configs"
	"github.com/jademperor/common/etcdutils"
	"github.com/jademperor/common/models"
	// "go.etcd.io/etcd/client"
)

// New Engine ...
func New(etcdAddrs []string, pluginsFlag []string, debug bool) (*Engine, error) {
	store, err := etcdutils.NewEtcdStore(etcdAddrs)
	if err != nil {
		return nil, err
	}

	e := &Engine{
		proxier:  proxy.New(nil, nil, nil),
		store:    store,
		debug:    debug,
		debugMux: http.NewServeMux(),
		// kapi:    kapi,
	}

	// proxier data loading ...
	e.prepare()

	e.initPlugins()
	e.installExtension(pluginsFlag)
	e.initialWatchers()

	// generate a new contextPool
	if e.contextPool, err = plugin.NewContextPool(10000, 100000,
		plugin.DefaultFactory, e.allPlugins); err != nil {
		return nil, err
	}

	// debug mode pprof
	if e.debug {
		e.debugMux.HandleFunc("/debug/pprof/", pprof.Index)
		e.debugMux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		e.debugMux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		e.debugMux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		e.debugMux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	}

	return e, nil
}

// Engine contains fields to server http server with http request
type Engine struct {
	allPlugins   []plugin.Plugin      // all register plugins
	numAllPlugin int                  // num of plugin
	proxier      *proxy.Proxier       // proxier
	store        *etcdutils.EtcdStore // etcd storer
	contextPool  *plugin.ContextPool
	debug        bool
	debugMux     *http.ServeMux
	// kapi         client.KeysAPI  // etcd client api
	// addr         string          // gate addr
}

// initial plugins
func (e *Engine) initPlugins() {
	plgHTTPLogger := httplog.New(logger.Logger)
	plgTokenBucket := ratelimit.New(1000000, 1000)

	// install plugins
	e.use(plgHTTPLogger)  // idx = 0
	e.use(plgTokenBucket) // idx = 1
}

func (e *Engine) installExtension(pluginsFlag []string) {
	for _, plgFlag := range pluginsFlag {
		plg, err := plugin.InstallExtension(plgFlag)
		if err != nil {
			logger.Logger.Errorf("plugin.InstallExtension() got error: %v, skip this", err)
			continue
		}
		if plg == nil {
			continue
		}
		e.use(plg)
	}
}

func (e *Engine) use(plgs ...plugin.Plugin) {
	e.allPlugins = append(e.allPlugins, plgs...)
	e.numAllPlugin += len(plgs)
}

func (e *Engine) prepare() {
	e.prepareClusters()
	e.prepareAPIs()
	e.prepareRoutings()
}

// prepare load clusters info and proxy models into Engine.proxier
func (e *Engine) prepareClusters() {
	var (
		clusterCfgs = make(map[string][]*models.ServerInstance)
	)

	e.store.Iter(configs.ClustersKey, 2, func(k, v string, dir bool) {
		if dir {
			return
		}

		// skip option nodes
		splitResults := strings.Split(k, "/")
		if splitResults[3] == configs.ClusterOptionsKey {
			return
		}

		logger.Logger.Info("find server instance: ", k)
		srvInsCfg := new(models.ServerInstance)
		if err := etcdutils.Decode(v, srvInsCfg); err != nil {
			logger.Logger.Error(err)
			return
		}
		if !srvInsCfg.IsAlive {
			return
		}
		clusterCfgs[splitResults[2]] = append(clusterCfgs[splitResults[2]], srvInsCfg)
		logger.Logger.Info("add an available instance: ", srvInsCfg)
	})

	e.proxier.LoadClusters(clusterCfgs)
	e.proxier.LoadBreakers(clusterCfgs)
}

func (e *Engine) prepareAPIs() {
	var (
		apiCfgs []*models.API
	)

	e.store.Iter(configs.APIsKey, 1, func(k, v string, dir bool) {
		if dir {
			return
		}
		logger.Logger.Info("find api cfg instance: ", k)
		apiCfg := new(models.API)
		etcdutils.Decode(v, apiCfg)
		apiCfgs = append(apiCfgs, apiCfg)
	})
	e.proxier.LoadAPIs(apiCfgs)
}

func (e *Engine) prepareRoutings() {
	var (
		routingCfgs = make([]*models.Routing, 0)
	)

	e.store.Iter(configs.RoutingsKey, 1, func(k, v string, dir bool) {
		if dir {
			return
		}
		logger.Logger.Info("find routing cfg instance: ", k)
		routingCfg := new(models.Routing)
		etcdutils.Decode(v, routingCfg)
		routingCfgs = append(routingCfgs, routingCfg)
	})
	e.proxier.LoadRouting(routingCfgs)
}

// ServeHTTP the implemention of http.Handler
func (e *Engine) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if e.debug && strings.HasPrefix(req.URL.Path, "/debug") {
		e.debugMux.ServeHTTP(w, req)
		return
	}
	// ctx := plugin.NewContext(w, req, e.allPlugins)
	ctx, err := e.contextPool.Get(w, req, plugin.DefaultPreFactory)
	if err != nil {
		ctx.SetError(err)
		return
	}

	// start process with plugin
	ctx.Next()

	if ctx.Aborted() {
		return
	}

	e.proxier.Handle(ctx)
	e.contextPool.Put(ctx)
}

// Run Engine start listenning and serving by ServeHTTP
func (e *Engine) Run(addr string) error {
	// e.init(addr)
	logger.Logger.WithFields(map[string]interface{}{
		"numPlugins": e.numAllPlugin,
		"addr":       addr,
	}).Info("start listening")

	timeout := 5 * time.Second
	if e.debug {
		timeout = time.Duration(100 * time.Second)
	}
	handler := http.TimeoutHandler(e, timeout, configs.TIMEOUT)
	return http.ListenAndServe(addr, handler)
}
