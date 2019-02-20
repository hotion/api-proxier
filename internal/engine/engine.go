package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jademperor/api-proxier/internal/logger"
	"github.com/jademperor/api-proxier/internal/plugin"
	"github.com/jademperor/api-proxier/internal/plugin/cache"
	"github.com/jademperor/api-proxier/internal/plugin/cache/presistence"
	"github.com/jademperor/api-proxier/internal/plugin/httplog"
	"github.com/jademperor/api-proxier/internal/plugin/ratelimit"
	// "github.com/jademperor/api-proxier/internal/plugin/rbac"
	"github.com/jademperor/api-proxier/internal/proxy"
	"github.com/jademperor/common/configs"
	"github.com/jademperor/common/etcdutils"
	"github.com/jademperor/common/models"
	"go.etcd.io/etcd/client"
)

// New Engine ...
func New(etcdAddrs []string) (*Engine, error) {
	kapi, err := etcdutils.Connect(etcdAddrs...)
	if err != nil {
		return nil, err
	}

	e := &Engine{
		proxier: proxy.New(nil, nil, nil),
		kapi:    kapi,
	}

	// proxier data loading ...
	e.prepare()

	e.initPlugins()
	e.initialWatchers()

	return e, nil
}

// Engine contains fields to server http server with http request
type Engine struct {
	allPlugins   []plugin.Plugin // all register plugins
	numAllPlugin int             // num of plugin
	proxier      *proxy.Proxier  // proxier
	kapi         client.KeysAPI  // etcd client api
	// addr         string          // gate addr
}

// initial plugins
func (e *Engine) initPlugins() {
	plgHTTPLogger := httplog.New(logger.Logger)
	plgCache := cache.New(presistence.NewInMemoryStore(), nil)
	plgTokenBucket := ratelimit.New(10, 1)
	// plgRBAC := rbac.New("user_id", nil, nil)

	// e.prepareRBAC(plgRBAC)
	e.prepareCache(plgCache)

	// install plugins
	e.use(plgHTTPLogger)  // idx = 0
	e.use(plgCache)       // idx = 1
	e.use(plgTokenBucket) // idx = 2
	// e.use(plgRBAC)        // idx = 3
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
	resp, err := e.kapi.Get(context.Background(), configs.ClustersKey, nil)
	if err != nil || !resp.Node.Dir {
		return
	}
	for _, clusterNode := range resp.Node.Nodes {
		clusterID := strings.Split(clusterNode.Key, "/")[2]
		srvInses := make([]*models.ServerInstance, 0)
		logger.Logger.Infof("find cluster instance id: %s", clusterID)
		if resp2, err := e.kapi.Get(context.Background(), clusterNode.Key, nil); err == nil && resp2.Node.Dir {
			for _, srvInsNode := range resp2.Node.Nodes {
				logger.Logger.Info("find server instance: ", srvInsNode.Key)
				// skip cluster option node
				if strings.Split(srvInsNode.Key, "/")[3] == configs.ClusterOptionsKey {
					continue
				}
				srvInsCfg := new(models.ServerInstance)
				if err := json.Unmarshal([]byte(srvInsNode.Value), srvInsCfg); err != nil {
					logger.Logger.Error(err)
					continue
				}
				srvInses = append(srvInses, srvInsCfg)
			}
			if len(srvInses) != 0 {
				clusterCfgs[clusterID] = srvInses
				logger.Logger.Infof("clusterCfgs register: id-%s, count-%d", clusterID, len(srvInses))
			}
		}
	}
	logger.Logger.Info(clusterCfgs)
	e.proxier.LoadClusters(clusterCfgs)
}

func (e *Engine) prepareAPIs() {
	var (
		apiCfgs []*models.API
	)
	resp, err := e.kapi.Get(context.Background(), configs.APIsKey, nil)
	if err != nil || !resp.Node.Dir {
		return
	}
	for _, apiNode := range resp.Node.Nodes {
		logger.Logger.Info("find api cfg instance: ", apiNode.Key)
		apiCfg := new(models.API)
		json.Unmarshal([]byte(apiNode.Value), apiCfg)
		apiCfgs = append(apiCfgs, apiCfg)
	}
	e.proxier.LoadAPIs(apiCfgs)
}

func (e *Engine) prepareRoutings() {
	var (
		routingCfgs = make([]*models.Routing, 0)
	)
	resp, err := e.kapi.Get(context.Background(), configs.RoutingsKey, nil)
	if err != nil || !resp.Node.Dir {
		return
	}
	for _, routingNode := range resp.Node.Nodes {
		logger.Logger.Info("find routing cfg instance: ", routingNode.Key)
		routingCfg := new(models.Routing)
		json.Unmarshal([]byte(routingNode.Value), routingCfg)
		routingCfgs = append(routingCfgs, routingCfg)
	}
	e.proxier.LoadRouting(routingCfgs)
}

// func (e *Engine) prepareRBAC() { }

func (e *Engine) prepareCache(c *cache.Cache) {
	var (
		rules = make([]*models.NocacheCfg, 0)
	)
	resp, err := e.kapi.Get(context.Background(), configs.RoutingsKey, nil)
	if err != nil || !resp.Node.Dir {
		return
	}
	for _, cacheNode := range resp.Node.Nodes {
		logger.Logger.Info("find routing cfg instance: ", cacheNode.Key)
		nocCfg := new(models.NocacheCfg)
		json.Unmarshal([]byte(cacheNode.Value), nocCfg)
		rules = append(rules, nocCfg)
	}
	c.Load(rules)
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
