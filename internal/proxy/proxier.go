// Package proxy ...
// this file mainly to load from file and set proxy rules
package proxy

import (
	"context"
	"errors"
	// "fmt"
	// "log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/jademperor/api-proxier/internal/logger"
	"github.com/jademperor/api-proxier/plugin"
	"github.com/jademperor/common/models"
	"github.com/jademperor/common/pkg/code"
	"github.com/jademperor/common/pkg/utils"
	"github.com/julienschmidt/httprouter"
	"github.com/sony/gobreaker"
	// roundrobin "github.com/jademperor/common/pkg/round-robin"
)

const (
	reverseKeyLayout = "%s_%d"
	ctxTIMEOUT       = time.Second * 5
)

var (
	// ErrPageNotFound can not found page
	ErrPageNotFound = errors.New("Page not found")
	// ErrNoAvailableCluster no
	ErrNoAvailableCluster = errors.New("No available cluster")
)

func defaultHandleFunc(w http.ResponseWriter, req *http.Request, params httprouter.Params) {}

func defaultErrorHandler(w http.ResponseWriter, req *http.Request, err error) {
	utils.ResponseJSON(w,
		code.NewCodeInfo(code.CodeSystemErr, err.Error()))
	return
}

// New ...
func New(
	apiRules []*models.API,
	reverseSrvs map[string][]*models.ServerInstance,
	routingRules []*models.Routing) *Proxier {

	p := &Proxier{
		mutex:  &sync.RWMutex{},
		router: httprouter.New(),
		status: plugin.Working,
	}

	// initial work
	p.LoadClusters(reverseSrvs)
	p.LoadAPIs(apiRules)
	p.LoadRouting(routingRules)
	p.LoadBreakers(reverseSrvs)

	return p
}

// Proxier the entity to math proxy rules and do proxy request to servers
type Proxier struct {
	mutex        *sync.RWMutex
	status       plugin.PlgStatus
	router       *httprouter.Router         // router to match incoming URL and Method
	clusters     map[string]*models.Cluster // clusters to manage reverseProxies
	apiRules     map[string]*models.API     // apis configs to proxy
	routingRules map[string]*models.Routing // routing configs to proxy

	cb map[string]*gobreaker.CircuitBreaker
}

// Handle proxy to handle with request
func (p *Proxier) Handle(c *plugin.Context) {
	defer plugin.Recover("Proxier")

	p.mutex.RLock()
	defer p.mutex.RUnlock()

	// match api reverse proxy
	if rule, ok := p.matchAPIRule(c.Method, c.Path); ok {
		logger.Logger.Debugln("matched path rules")
		if rule.NeedCombine {
			if err := p.callAPIWithCombination(rule, c); err != nil {
				c.SetError(err)
				c.AbortWithStatus(http.StatusInternalServerError)
			}
		} else {
			if err := p.callAPI(rule, c); err != nil {
				c.SetError(err)
				c.AbortWithStatus(http.StatusInternalServerError)
			}
		}
		return
	}

	// match routing proxy
	if rule, ok := p.matchRoutingRule(c.Path); ok {
		logger.Logger.Debugln("matched server rules")
		if err := p.callRouting(rule, c); err != nil {
			c.SetError(err)
			c.AbortWithStatus(http.StatusInternalServerError)
		}
		return
	}

	// don't matched any path or server !!!
	logger.Logger.Infof("could not match API or Routing rule with (method: %s, path: %s)",
		c.Method, c.Path)
	c.SetError(ErrPageNotFound)
	c.AbortWithStatus(http.StatusNotFound)
	return
}

// Status ...
func (p *Proxier) Status() plugin.PlgStatus {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	status := p.status
	return status
}

func (p *Proxier) matchAPIRule(method, path string) (*models.API, bool) {
	if handle, params, tsr := p.router.Lookup(method, path); handle != nil {
		_, _ = params, tsr
		return p.apiRules[path], true
	}
	return nil, false
}

func (p *Proxier) matchRoutingRule(path string) (*models.Routing, bool) {
	pathPrefix := utils.ParseURIPrefix(path)
	pathPrefix = strings.ToLower(pathPrefix)
	rule, ok := p.routingRules[pathPrefix]
	return rule, ok
}

// LoadBreakers ...
func (p *Proxier) LoadBreakers(cfgs map[string][]*models.ServerInstance) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.cb = make(map[string]*gobreaker.CircuitBreaker)
	var cbSt gobreaker.Settings

	for clsID, cls := range cfgs {
		for _, ins := range cls {
			if !ins.OpenBreaker {
				continue
			}

			// TODO(done): support ins has config of breaker settings
			if ins.BreakerSetting != nil {
				// has own breaker setting
				cbSt = gobreaker.Settings{
					Name:        genCbKey(clsID, ins.Idx),
					Interval:    time.Duration(ins.BreakerSetting.ClearInterval) * time.Millisecond,
					MaxRequests: ins.BreakerSetting.MaxRequests,
					Timeout:     time.Duration(ins.BreakerSetting.Timeout) * time.Millisecond,
					ReadyToTrip: genReadyToTrip(ins.BreakerSetting.TripRequestCnt,
						ins.BreakerSetting.TripFailureRatio),
				}
				logger.Logger.Infof("breaker %s is registered with setting: %v", cbSt.Name, cbSt)
			} else {
				// set with default setting
				cbSt = gobreaker.Settings{
					Name:        genCbKey(clsID, ins.Idx),
					ReadyToTrip: defaultReadyToTrip,
				}
				logger.Logger.Infof("breaker %s is registered with default setting", cbSt.Name)
			}
			p.cb[cbSt.Name] = gobreaker.NewCircuitBreaker(cbSt)
		}
	}
}

// LoadClusters to load cfgs (type []proxy.ReverseServerCfg) to initial Proxier.Balancers
func (p *Proxier) LoadClusters(cfgs map[string][]*models.ServerInstance) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.clusters = make(map[string]*models.Cluster)
	for clsID, cfg := range cfgs {
		// ignore empty cluster
		if len(cfg) != 0 {
			p.clusters[clsID] = models.NewCluster(clsID, "", cfgs[clsID])
			logger.Logger.Infof("cluster :%s, registered with %d instance", clsID, len(cfg))
		}
	}
}

// LoadAPIs to load rules (type []proxy.PathRule) to initial
func (p *Proxier) LoadAPIs(rules []*models.API) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.apiRules = make(map[string]*models.API)
	p.router = httprouter.New()
	for _, rule := range rules {
		// [TODO](done): valid rule all string need to be lower
		path := strings.ToLower(rule.Path)
		method := strings.ToLower(rule.Method)
		if _, ok := p.apiRules[path]; ok {
			panic(utils.Fstring("duplicate path rule: %s", path))
		}
		p.apiRules[path] = rule
		for _, method := range strings.Split(rule.Method, ",") {
			p.router.Handle(method, path, defaultHandleFunc)
		}

		logger.Logger.Infof("URI rule:%s_%s registered", path, method)
	}
}

// LoadRouting to load rules (type []proxy.ServerRule) to initial
func (p *Proxier) LoadRouting(rules []*models.Routing) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.routingRules = make(map[string]*models.Routing)
	for _, rule := range rules {
		// [TODO](done): valid rule all string need to be lower
		prefix := strings.ToLower(rule.Prefix)
		if len(prefix) <= 1 {
			logger.Logger.Errorf("prefix of [%s] is invalid , so skipped\n", prefix)
			continue
		}
		if prefix[0] != '/' {
			prefix = "/" + prefix
		}

		if _, ok := p.routingRules[prefix]; ok {
			panic(utils.Fstring("duplicate server rule prefix: %s", prefix))
		}
		p.routingRules[prefix] = rule
		logger.Logger.Infof("SRV rule: [%s_%s] registered", rule.ClusterID, rule.Prefix)
	}
}

// callAPIWithCombination
// [TODO](done): combine two or more response
func (p *Proxier) callAPIWithCombination(rule *models.API, c *plugin.Context) error {
	respChan := make(chan responseChan, len(rule.CombineReqCfgs))
	wg := sync.WaitGroup{}
	ctx, cancel := context.WithTimeout(context.Background(), ctxTIMEOUT)
	defer cancel()

	for _, combCfg := range rule.CombineReqCfgs {
		wg.Add(1)
		go func(comb *models.APICombination, respC chan<- responseChan) {
			defer wg.Done()
			cls, ok := p.clusters[comb.TargetClusterID]
			if !ok {
				respC <- responseChan{Err: ErrNoAvailableCluster, Field: comb.Field, Data: nil}
				return
			}
			srvIns := cls.Distribute()

			// cb pipeline
			cb, exist := p.cb[genCbKey(cls.Idx, srvIns.Idx)]
			logger.Logger.Debugf("got cb with key: %s, got: %v", genCbKey(cls.Idx, srvIns.Idx), ok)

			if !exist {
				// none breaker execute
				combineReq(ctx, srvIns.Addr, nil, comb, respC)
			} else {
				// breaker execute
				if _, err := cb.Execute(func() (interface{}, error) {
					combineReq(ctx, srvIns.Addr, nil, comb, respC)
					return nil, nil
				}); err != nil {
					respC <- responseChan{Err: err, Field: comb.Field, Data: nil}
				}
			}
		}(combCfg, respChan)
	}

	wg.Wait()
	close(respChan)

	final := map[string]interface{}{
		"code":    0,
		"message": "OK",
	}

	// loop response combine to togger response
	for r := range respChan {
		if r.Err != nil {
			final[r.Field] = r.Err.Error()
			continue
		}
		// read response
		final[r.Field] = r.Data
	}

	// Response
	c.JSON(http.StatusOK, final)
	return nil
}

// callAPI reverse proxy to remote server and combine repsonse.
func (p *Proxier) callAPI(rule *models.API, c *plugin.Context) error {
	oriPath := strings.ToLower(rule.Path)
	req := c.Request()
	w := c.ResponseWriter()

	if len(rule.RewritePath) != 0 {
		req.URL.Path = rule.RewritePath
	}

	clsID := strings.ToLower(rule.TargetClusterID)
	cls, ok := p.clusters[clsID]
	if !ok {
		logger.Logger.Errorf("could not found balancer [%s], and target_cls_id [%s]", oriPath, clsID)
		return ErrNoAvailableCluster
	}
	srvIns := cls.Distribute()

	// [TODO](done): prevent requets
	// execute proxy call
	cb, exist := p.cb[genCbKey(cls.Idx, srvIns.Idx)]
	logger.Logger.Debugf("got cb with key: %s, got: %v", genCbKey(cls.Idx, srvIns.Idx), ok)

	var (
		err error
	)

	if !exist {
		reverseProxy := generateReverseProxy(srvIns)
		reverseProxy.ErrorHandler = defaultErrorHandler
		reverseProxy.ServeHTTP(w, req)
	} else {
		// cb work pipe
		_, err = cb.Execute(func() (v interface{}, err1 error) {
			reverseProxy := generateReverseProxy(srvIns)
			reverseProxy.ErrorHandler = func(w http.ResponseWriter, req *http.Request, err0 error) {
				err1 = err0
				// defaultErrorHandler(w, req, e)
			}
			reverseProxy.ServeHTTP(w, req)
			return nil, err1
		})
	}
	return err
}

// callRouting to proxy request to another server
// cannot combine two server response
func (p *Proxier) callRouting(rule *models.Routing, c *plugin.Context) error {
	// need to trim prefix
	req := c.Request()
	w := c.ResponseWriter()
	if rule.NeedStripPrefix {
		req.URL.Path = strings.TrimPrefix(strings.ToLower(req.URL.Path),
			strings.ToLower(rule.Prefix))
	}

	clsID := strings.ToLower(rule.ClusterID)
	cls, ok := p.clusters[clsID]

	if !ok {
		logger.Logger.Errorf("%s Not Found!", clsID)
		return ErrNoAvailableCluster
	}

	srvIns := cls.Distribute()
	// setRequestWithInstanceID(req, cls.Idx, srvIns.Idx)

	// execute proxy call
	// [TODO](done): preventRequest
	cb, exist := p.cb[genCbKey(cls.Idx, srvIns.Idx)]
	logger.Logger.Debugf("got cb with key: %s, got: %v", genCbKey(cls.Idx, srvIns.Idx), ok)

	var (
		err error
	)
	if !exist {
		reverseProxy := generateReverseProxy(srvIns)
		reverseProxy.ErrorHandler = defaultErrorHandler
		reverseProxy.ServeHTTP(w, req)

	} else {
		// cb work pipe
		_, err = cb.Execute(func() (v interface{}, err1 error) {
			reverseProxy := generateReverseProxy(srvIns)
			reverseProxy.ErrorHandler = func(w http.ResponseWriter, req *http.Request, err0 error) {
				err1 = err0
				// defaultErrorHandler(w, req, e)
			}
			reverseProxy.ServeHTTP(w, req)
			return nil, err1
		})

	}
	return err
}

// generateReverseProxy ...
// TODO: with cache
func generateReverseProxy(ins *models.ServerInstance) *httputil.ReverseProxy {
	target, err := url.Parse(ins.Addr)
	if err != nil {
		panic(utils.Fstring("could not parse URL: %s", ins.Addr))
	}
	reverseProxy := httputil.NewSingleHostReverseProxy(target)
	return reverseProxy
}

// const (
// 	xHeaderKey = "X-Instance-Key"
// )

// // to add a header named xHeaderKey with instance key
// // key = genCbKey(clsID, instanceID)
// func setRequestWithInstanceID(req *http.Request, clsID, instanceID string) {
// 	req.Header.Add(xHeaderKey, genCbKey(clsID, instanceID))
// }

func genCbKey(clsID, insID string) string {
	return clsID + "_" + insID
}
