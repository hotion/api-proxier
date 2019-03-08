package plugin

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"sync"

	"github.com/jademperor/common/pkg/utils"
)

var (
	errPoolClosed     = errors.New("pool has been closed")
	errInvalidContext = errors.New("plugin.Context is nil. rejecting")

	errEmptyContext        = errors.New("ctx is nil. rejecting")
	errEmptyResponseWriter = errors.New("w is nil. rejecting")
	errEmptyRequest        = errors.New("req is nil. rejecting")
)

// ContextPool generate Context from pool
type ContextPool struct {
	mutex       sync.RWMutex
	ctxChanPool chan *Context
	f           GenFactory
	pluginsCpy  []Plugin
}

// GenFactory generate a Context object, notice that:
// while pool is initializing, w and req is nil for sure
type GenFactory func(w http.ResponseWriter,
	req *http.Request, plugins []Plugin) (*Context, error)

// PreFactory to prepare a Context with w and req entity
type PreFactory func(ctx *Context,
	w http.ResponseWriter, req *http.Request) error

// DefaultFactory for ContextPool to generate a Context
func DefaultFactory(w http.ResponseWriter,
	req *http.Request, plugins []Plugin) (*Context, error) {
	var (
		method, path string
		cpyReq       *http.Request
		cCtx         context.Context
		form         url.Values
	)

	if req != nil {
		method = req.Method
		path = req.URL.Path
		cpyReq = utils.CopyRequest(req)
		cCtx = req.Context()
		form = utils.ParseRequestForm(cpyReq)
	}

	if w != nil {
		// do nothing for now
	}

	return &Context{
		Ctx:       cCtx,
		Method:    method,
		Path:      path,
		Form:      form,
		plugins:   plugins,
		numPlugin: len(plugins),
		pluginIdx: -1,
		w:         w,
		req:       req,
		aborted:   false,
	}, nil
}

// DefaultPreFactory to implements PreFactory
func DefaultPreFactory(ctx *Context, w http.ResponseWriter,
	req *http.Request) error {
	if ctx == nil {
		return errEmptyContext
	}
	if w == nil {
		return errEmptyResponseWriter
	}
	if req == nil {
		return errEmptyRequest
	}

	ctx.w = w

	ctx.Method = req.Method
	ctx.Path = req.URL.Path
	ctx.Ctx = req.Context()

	cpyReq := utils.CopyRequest(req)
	ctx.req = cpyReq
	ctx.Form = utils.ParseRequestForm(cpyReq)

	return nil
}

// NewContextPool new a pool of context
func NewContextPool(initialCap, maxCap int, f GenFactory, plugins []Plugin) (*ContextPool, error) {
	cpool := &ContextPool{
		mutex:       sync.RWMutex{},
		ctxChanPool: make(chan *Context, maxCap),
		f:           f,
		pluginsCpy:  plugins,
	}

	for i := 0; i < initialCap; i++ {
		ctx, err := f(nil, nil, plugins)
		if err != nil {
			cpool.Close()
			return nil, err
		}
		cpool.ctxChanPool <- ctx
	}

	return cpool, nil
}

// Close pool with resoure releasing
func (pool *ContextPool) Close() {
	pool.mutex.Lock()
	ctxChan := pool.ctxChanPool
	pool.ctxChanPool = nil
	pool.f = nil
	pool.mutex.Unlock()

	if ctxChan == nil {
		return
	}

	close(ctxChan)
	for c := range ctxChan {
		c.close()
	}
}

func (pool *ContextPool) getCtxChanPoolAndFactory() (chan *Context, GenFactory, []Plugin) {
	pool.mutex.RLock()
	ctxChan, f, plugins := pool.ctxChanPool, pool.f, pool.pluginsCpy
	pool.mutex.RUnlock()
	return ctxChan, f, plugins
}

// Get a Context from pool
func (pool *ContextPool) Get(w http.ResponseWriter, req *http.Request, prefunc PreFactory) (
	ctx *Context, err error) {
	ctxChan, f, plugins := pool.getCtxChanPoolAndFactory()

	if ctxChan == nil {
		err = errPoolClosed
		return
	}

	select {
	case ctx = <-ctxChan:
		if ctx == nil {
			err = errPoolClosed
			return
		}
		// prepare ctx
		prefunc(ctx, w, req)
	default:
		ctx, err = f(w, req, plugins)
	}
	return
}

// Put a context back
func (pool *ContextPool) Put(ctx *Context) error {
	if ctx == nil {
		return errInvalidContext
	}
	ctx.Reset()

	pool.mutex.RLock()
	defer pool.mutex.RUnlock()

	if pool.ctxChanPool == nil {
		// pool is closed, close passed connection
		ctx.close()
		return nil
	}

	// put the resource back into the pool. If the pool is full, this will
	// block and the default case will be executed.
	select {
	case pool.ctxChanPool <- ctx:
	default:
		// pool is full, close passed context
		ctx.close()
	}
	return nil
}
