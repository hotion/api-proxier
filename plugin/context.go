package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/jademperor/common/pkg/code"
	"github.com/jademperor/common/pkg/utils"
)

// var _ Ctx = &Context{}

// Ctx for proxier to transfer request body and other
// type Ctx interface {
// 	Next()
// 	Abort(status int)
// 	Aborted() bool
// 	String(s string) error
// 	JSON(v interface{}) error
// }

// NewContext generate a Context
// [TODO](done): do this work with pool, but here remain this funcs
// [TODO](done): design a test for this function with ContextPool
func NewContext(w http.ResponseWriter, req *http.Request, plugins []Plugin) *Context {
	method := req.Method
	path := req.URL.Path
	cpyReq := utils.CopyRequest(req)

	return &Context{
		Ctx:       req.Context(),
		Method:    method,
		Path:      path,
		Form:      utils.ParseRequestForm(cpyReq),
		plugins:   plugins,
		numPlugin: len(plugins),
		pluginIdx: -1,
		w:         w,
		req:       req,
		aborted:   false,
	}
}

// Context contains infomation to transfer data between plugins
type Context struct {

	// Ctx means ctx control signal for multi goroutine
	Ctx context.Context
	// Method means request method
	Method string
	// Path means request Path
	Path string
	// Form includes current http request has been parsed form values
	Form url.Values

	req *http.Request
	w   http.ResponseWriter

	plugins   []Plugin
	pluginIdx int
	numPlugin int

	aborted bool  // request aborted
	err     error // error
}

// Next call next plugin in context, if has beed aborted then just return
func (c *Context) Next() {
	// fmt.Printf("plugin idx: %d, handle result: %v\n", c.pluginIdx, c.aborted)
	// handle aborrted
	if c.aborted {
		return
	}

	// call next
	c.pluginIdx++
	if c.pluginIdx >= c.numPlugin {
		return
	}

	if c.plugins[c.pluginIdx].Enabled() {
		c.plugins[c.pluginIdx].Handle(c)
	}
	c.Next()
}

// Abort process to stop calling next plugin
// [TODO](done): ignore response here, should call JSON, or String manually
func (c *Context) Abort() {
	c.aborted = true
}

// AbortWithStatus abort process and set response status
func (c *Context) AbortWithStatus(status int) {
	c.aborted = true
	c.w.WriteHeader(status)
}

// Aborted ...
func (c *Context) Aborted() bool {
	return c.aborted
}

// Set set request and  responseWriter
func (c *Context) Set(req *http.Request, w http.ResponseWriter) {
	c.req = req
	c.w = w
	c.pluginIdx = -1
}

// Reset ... donot call this manually
func (c *Context) Reset() {
	c.req = nil
	c.w = nil
	c.Form = nil
	c.aborted = false
	c.err = nil
	c.pluginIdx = -1
}

// Error get the global error of context
func (c *Context) Error() error {
	return c.err
}

// SetError set err as context error, but not abort the context procedure
func (c *Context) SetError(err error) {
	c.err = err
	c.JSON(http.StatusInternalServerError,
		code.NewCodeInfo(code.CodeSystemErr, err.Error()))
}

// Request ...
func (c *Context) Request() *http.Request {
	return c.req
}

// ResponseWriter ...
func (c *Context) ResponseWriter() http.ResponseWriter {
	return c.w
}

// SetResponseWriter ...
func (c *Context) SetResponseWriter(w http.ResponseWriter) {
	c.w = w
}

// JSON ...
func (c *Context) JSON(status int, v interface{}) {
	byts, err := json.Marshal(v)
	if err != nil {
		c.SetError(err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	c.w.Header().Set("Content-Type", "application/json")
	c.AbortWithStatus(status)
	fmt.Fprintf(c.w, string(byts))
}

// String ...
func (c *Context) String(status int, s string) {
	c.w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	c.AbortWithStatus(status)
	fmt.Fprintf(c.w, s)
}

// close only for ContextPool
func (c *Context) close() {}
