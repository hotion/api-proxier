package plugin_test

import (
	"github.com/jademperor/api-proxier/internal/stdplugin/httplog"
	"github.com/jademperor/api-proxier/internal/stdplugin/ratelimit"
	"github.com/jademperor/api-proxier/plugin"

	"net/http/httptest"
	"testing"
)

/*
	pkg: github.com/jademperor/api-proxier/plugin
	Benchmark_NewContextWithPool-4   	 1000000	      1937 ns/op	    1624 B/op	      14 allocs/op
	PASS
	ok  	github.com/jademperor/api-proxier/plugin	1.981s
*/
func Benchmark_NewContextWithPool(b *testing.B) {
	b.StopTimer()
	plugins := []plugin.Plugin{
		ratelimit.New(10, 10),
		httplog.New(nil),
	}
	pool, err := plugin.NewContextPool(50, 1000, plugin.DefaultFactory, plugins)
	if err != nil {
		b.Errorf("could not new Context Pool: %v", err)
	}

	req := httptest.NewRequest("GET", "http://example.com/foo", nil)
	w := httptest.NewRecorder()

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		ctx, _ := pool.Get(w, req, plugin.DefaultPreFactory)
		_ = ctx
	}
}
