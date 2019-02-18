// Package proxy ...
// this file mainly to load from file and set proxy rules
package proxy

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/jademperor/api-proxier/internal/logger"
	"github.com/jademperor/api-proxier/internal/plugin"
	"github.com/jademperor/common/configs"
	"github.com/jademperor/common/pkg/code"
	"github.com/jademperor/common/pkg/utils"
	// roundrobin "github.com/jademperor/common/pkg/round-robin"
)

const (
	reverseKeyLayout = "%s_%d"
	ctxTIMEOUT       = time.Second * 5
)

var (
	// ErrBalancerNotMatched plugin.Proxier balancer not matched
	ErrBalancerNotMatched = errors.New("plugin.Proxier balancer not matched")
	// ErrPageNotFound can not found page
	ErrPageNotFound = errors.New("404 Page Not Found")
	// ErrNoReverseServer ...
	ErrNoReverseServer = errors.New("could not found reverse proxy")
)

func defaultHandleFunc(w http.ResponseWriter, req *http.Request, params httprouter.Params) {}
func defaultErrorHandler(w http.ResponseWriter, req *http.Request, err error) {
	utils.ResponseJSON(w, 
		code.NewCodeInfo(code.CodeSystemErr, err.Error()))
	return
}


// New ...
func New(
	apiRules []*configs.API, 
	reverseSrvs map[string][]*configs.ServerInstance,
	routingRules []*configs.Routing) *Proxier{
	p := &Proxier{
		mutex: &sync.RWMutex{},
		router:  httprouter.New(),
		status:  plugin.Working,
	}

	// initial work
	p.LoadClusters(reverseSrvs)
	p.LoadAPIs(apiRules)
	p.LoadRouting(routingRules)

	return p
}

// Proxier ...
type Proxier struct {
	mutex *sync.RWMutex
	status  plugin.PlgStatus
	clusters map[string]*configs.Cluster // clusters to manage reverseProxies 
	router *httprouter.Router // router to match url
	apiRules map[string]*configs.API // apis configs
	routingRules map[string]*configs.Routing // routing configs
}
// Handle proxy to handle with request
func (p *Proxier) Handle(c *plugin.Context) {
	defer plugin.Recover("Proxier")

	// match api reverse proxy
	if rule, ok := p.matchAPIRule(c.Method, c.Path); ok {
		logger.Logger.Info("matched path rules")
		if err := p.callReverseURI(rule, c); err != nil {
			c.SetError(err)
			c.AbortWithStatus(http.StatusInternalServerError)
		}
		return
	}

	// match routing proxy
	if rule, ok := p.matchRoutingRule(c.Path); ok {
		logger.Logger.Info("matched server rules")
		if err := p.callReverseServer(rule, c); err != nil {
			c.SetError(err)
			c.AbortWithStatus(http.StatusInternalServerError)
		}
		return
	}

	// don't matched any path or server !!!
	logger.Logger.Infof("could not match path or server rule !!! (method: %s, path: %s)",
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


func (p *Proxier) matchAPIRule(method, path string) (*configs.API, bool) {
	if handle, params, tsr := p.router.Lookup(method, path); handle != nil {
		_, _ = params, tsr
		return p.apiRules[path], true
	}
	return nil, false
}

func (p *Proxier) matchRoutingRule(path string) (*configs.Routing, bool) {
	pathPrefix := utils.ParseURIPrefix(path)
	pathPrefix = strings.ToLower(pathPrefix)
	rule, ok := p.routingRules[pathPrefix]
	return rule, ok
}

// LoadClusters to load cfgs (type []proxy.ReverseServerCfg) to initial Proxier.Balancers
func (p *Proxier) LoadClusters(cfgs map[string][]*configs.ServerInstance) {
	p.clusters = make(map[string]*configs.Cluster, len(cfgs))
	for clsID, cfg := range cfgs {
		// ignore empty cluster
		if len(cfg) != 0 {
			p.clusters[clsID] = configs.NewCluster(cfgs[clsID])
		}
	}
}

// LoadAPIs to load rules (type []proxy.PathRule) to initial
func (p *Proxier) LoadAPIs(rules []*configs.API) {
	p.apiRules = make(map[string]*configs.API)
	p.router = httprouter.New()
	for _, rule := range rules {
		// [done] TODO: valid rule all string need to be lower
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
func (p *Proxier) LoadRouting(rules []*configs.Routing) {
	p.routingRules = make(map[string]*configs.Routing)
	for _, rule := range rules {
		// [done] TODO: valid rule all string need to be lower
		prefix := strings.ToLower(rule.Prefix)
		if len(prefix) <= 1 {
			log.Printf("error: prefix of %s is too short, so skipped\n", prefix)
			continue
		}
		if prefix[0] != '/' {
			prefix = "/" + prefix
		}

		if _, ok := p.routingRules[prefix]; ok {
			panic(utils.Fstring("duplicate server rule prefix: %s", prefix))
		}
		p.routingRules[prefix] = rule
		logger.Logger.Infof("SRV rule:%s_%s registered", rule.ClusterID, rule.Prefix)
	}
}

// callReverseURI reverse proxy to remote server and combine repsonse.
func (p *Proxier) callReverseURI(rule *configs.API, c *plugin.Context) error {
	oriPath := strings.ToLower(rule.Path)
	req := c.Request()
	w := c.ResponseWriter()
	// pure reverse proxy here
	if !rule.NeedCombine {
		if len(rule.RewritePath) != 0 {
			req.URL.Path = rule.RewritePath
		}

		clsID := strings.ToLower(rule.ClusterID)
		cls, ok := p.clusters[clsID]
		if !ok {
			logger.Logger.Errorf("could not found balancer of %s, %s", oriPath, clsID)
			errmsg := utils.Fstring("error: plugin.Proxier balancer not found! (path: %s)", oriPath)
			return fmt.Errorf("%v", errmsg)
		}

		srvIns := cls.Distribute()
		reverseProxy := generateReverseProxy(srvIns) 
		reverseProxy.ServeHTTP(w, req)
		return nil
	}

	// [done] TODO: combine two or more response
	respChan := make(chan responseChan, len(rule.CombineReqCfgs))
	ctx, cancel := context.WithTimeout(context.Background(), ctxTIMEOUT)
	defer cancel()

	wg := sync.WaitGroup{}
	for _, combCfg := range rule.CombineReqCfgs {
		wg.Add(1)
		go func(comb *configs.APICombination) {
			defer wg.Done()
			cls, ok := p.clusters[comb.ServerName]
			if !ok {
				respChan <- responseChan{
					Err:   ErrBalancerNotMatched,
					Field: comb.Field,
					Data:  nil,
				}
				return
			}
			srvIns := cls.Distribute()
			combineReq(ctx, srvIns.Addr, nil, comb, respChan)
		}(combCfg)
	}

	wg.Wait()
	close(respChan)

	r := map[string]interface{}{
		"code":    0,
		"message": "combine result",
	}

	// loop response combine to togger response
	for resp := range respChan {
		if resp.Err != nil {
			r[resp.Field] = resp.Err.Error()
			continue
		}
		// read response
		r[resp.Field] = resp.Data
	}

	// Response
	c.JSON(http.StatusOK, r)

	return nil
}

// callReverseServer to proxy request to another server
// cannot combine two server response
func (p *Proxier) callReverseServer(rule *configs.Routing, c *plugin.Context) error {
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
		errmsg := utils.Fstring("%s Not Found!", clsID)
		return fmt.Errorf("%v", errmsg)
	}

	srvIns := cls.Distribute()
	reverseProxy := generateReverseProxy(srvIns)
	logger.Logger.Infof("proxy to %s", req.URL.Path)
	reverseProxy.ServeHTTP(w, req)
	return nil
}

// generateReverseProxy ...
// TODO: with cache
func generateReverseProxy(ins *configs.ServerInstance) *httputil.ReverseProxy {
	target, err := url.Parse(ins.Addr)
	if err != nil {
		panic(utils.Fstring("could not parse URL: %s", ins.Addr))
	}
	reversePorxy := httputil.NewSingleHostReverseProxy(target)
	// register a func for reverse proxy to handler error
	reversePorxy.ErrorHandler = defaultErrorHandler
	return reversePorxy
}