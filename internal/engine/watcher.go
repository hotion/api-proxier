package engine

import (
	"sync"
	"time"

	"github.com/jademperor/api-proxier/internal/logger"
	// "github.com/jademperor/api-proxier/plugin/cache"
	"github.com/jademperor/common/configs"
	"github.com/jademperor/common/etcdutils"
	"github.com/jademperor/common/pkg/utils"
)

var (
	clusterWatcher  *etcdutils.Watcher // cluster watcher
	apisWatcher     *etcdutils.Watcher
	routingsWatcher *etcdutils.Watcher
	// cacheWatcher    *etcdutils.Watcher // cache watcher
	// rbacWatcher     *etcdutils.Watcher // rabc plugin watcher
	// etc

	defaultDuration = 2 * time.Second
	hashCache       sync.Map // to store etcd key with value be hashed string
)

// initialWatchers to get all watchers to be ready of watching the change of config
func (e *Engine) initialWatchers() {
	clusterWatcher = etcdutils.NewWatcher(e.store.Kapi, defaultDuration, configs.ClustersKey)
	apisWatcher = etcdutils.NewWatcher(e.store.Kapi, defaultDuration, configs.APIsKey)
	routingsWatcher = etcdutils.NewWatcher(e.store.Kapi, defaultDuration, configs.RoutingsKey)
	// rbacWatcher = etcdutils.NewWatcher(e.store.Kapi, defaultDuration, configs.RbacKey)
	// cacheWatcher = etcdutils.NewWatcher(e.store.Kapi, defaultDuration, configs.CacheKey)

	go clusterWatcher.Watch(e.clusterCallback)
	go apisWatcher.Watch(e.apisCallback)
	go routingsWatcher.Watch(e.routingsCallback)
	// go cacheWatcher.Watch(e.cacheCallback)
	// go rbacWatcher.Watch(e.rbacCallback)
}

func (e *Engine) clusterCallback(op etcdutils.OpCode, k, v string) {
	// logger.Logger.Infof("clusters Op: %d, key: %s, value: %s", op, k, v)
	h := utils.StringMD5(v)

	// only if loaded(true) and not changed, can skip
	// it means 'k' has been exist and 'h' no change
	if actual, loaded := hashCache.LoadOrStore(k, h); loaded && h == actual.(string) {
		return
	}

	// else store it and reload clusters
	hashCache.Store(k, h)
	logger.Logger.Info("reload cluster configs")
	e.prepareClusters()
}

func (e *Engine) apisCallback(op etcdutils.OpCode, k, v string) {
	logger.Logger.Infof("apis Op: %d, key: %s, value: %s", op, k, v)
	e.prepareAPIs()
}

func (e *Engine) routingsCallback(op etcdutils.OpCode, k, v string) {
	logger.Logger.Infof("routings Op: %d, key: %s, value: %s", op, k, v)
	e.prepareRoutings()
}
