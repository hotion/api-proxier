package plugin_test

import (
	"github.com/jademperor/api-proxier/plugin"
	"github.com/jademperor/api-proxier/plugin/httplog"
	"github.com/jademperor/api-proxier/plugin/ratelimit"

	"net/http/httptest"
	"testing"
)

/*
pkg: github.com/jademperor/api-proxier/plugin
Benchmark_NewContextWithoutPool-4   	 1000000	      1761 ns/op	    1624 B/op	      15 allocs/op
PASS
ok  	github.com/jademperor/api-proxier/plugin	2.577s
*/
func Benchmark_NewContextWithoutPool(b *testing.B) {
	b.StopTimer()
	plugins := []plugin.Plugin{
		ratelimit.New(10, 10),
		httplog.New(nil),
	}

	req := httptest.NewRequest("GET", "http://example.com/foo", nil)
	w := httptest.NewRecorder()

	b.StartTimer()

	for i := 0; i < b.N; i++ {
		ctx := plugin.NewContext(w, req, plugins)
		_ = ctx
	}
}
