package engine

import (
	// "context"
	// "encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jademperor/api-proxier/internal/logger"
	"github.com/jademperor/api-proxier/internal/proxy"
	"github.com/jademperor/api-proxier/plugin"
	// "github.com/jademperor/api-proxier/plugin/cache"
	// "github.com/jademperor/api-proxier/plugin/cache/presistence"
	"github.com/jademperor/api-proxier/plugin/httplog"
	"github.com/jademperor/api-proxier/plugin/ratelimit"
	"github.com/jademperor/common/configs"
	"github.com/jademperor/common/etcdutils"
	"github.com/jademperor/common/models"
	// "go.etcd.io/etcd/client"
)

// New Engine ...
func New(etcdAddrs []string, pluginsFlag []string) (*Engine, error) {
	store, err := etcdutils.NewEtcdStore(etcdAddrs)
	if err != nil {
		return nil, err
	}

	e := &Engine{
		proxier: proxy.New(nil, nil, nil),
		store:   store,
		// kapi:    kapi,
	}

	// proxier data loading ...
	e.prepare()

	e.initPlugins()
	e.installExtension(pluginsFlag)
	e.initialWatchers()

	return e, nil
}

// Engine contains fields to server http server with http request
type Engine struct {
	allPlugins   []plugin.Plugin      // all register plugins
	numAllPlugin int                  // num of plugin
	proxier      *proxy.Proxier       // proxier
	store        *etcdutils.EtcdStore // etcd storer
	// kapi         client.KeysAPI  // etcd client api
	// addr         string          // gate addr
}

// initial plugins
func (e *Engine) initPlugins() {
	plgHTTPLogger := httplog.New(logger.Logger)
	plgTokenBucket := ratelimit.New(10, 1)

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
	ctx := plugin.NewContext(w, req, e.allPlugins)
	ctx.Next()

	if ctx.Aborted() {
		return
	}

	e.proxier.Handle(ctx)
}

// Run Engine start listenning and serving by ServeHTTP
func (e *Engine) Run(addr string) error {
	// e.init(addr)
	logger.Logger.WithFields(map[string]interface{}{
		"numPlugins": e.numAllPlugin,
		"addr":       addr,
	}).Info("start listening")

	handler := http.TimeoutHandler(e, 5*time.Second, configs.TIMEOUT)
	return http.ListenAndServe(addr, handler)
}
