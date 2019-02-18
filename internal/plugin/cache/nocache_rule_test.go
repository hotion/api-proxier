package cache

import (
	"testing"

	"github.com/jademperor/common/configs"
)

func Test_initRules(t *testing.T) {
	c := &Cache{}

	type args struct {
		rules []*configs.NocacheCfg
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "case 1",
			args: args{
				rules: []*configs.NocacheCfg{
					&configs.NocacheCfg{Regexp: "^/api$"},
					&configs.NocacheCfg{Regexp: "/d{1,2}*"},
				},
			},
		},
		{
			name: "case 2",
			args: args{
				rules: []*configs.NocacheCfg{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c.Load(tt.args.rules)
			if want := len(tt.args.rules); c.cntRegexp != want {
				t.Errorf("could not initRules, not equal length: %d, want %d",
					c.cntRegexp, want)
			}
		})
	}
}

func Test_matchNoCacheRule(t *testing.T) {
	c := &Cache{}
	c.Load([]*configs.NocacheCfg{
		&configs.NocacheCfg{Regexp: "^/api/url$"},
		&configs.NocacheCfg{Regexp: "^/api/test$"},
		&configs.NocacheCfg{Regexp: "^/api/hire$"},
	})

	type args struct {
		uri string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "case 1",
			args: args{
				uri: "/api/url",
			},
			want: true,
		},
		{
			name: "case 1",
			args: args{
				uri: "/api/hhhh/ashdak",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := c.matchNoCacheRule(tt.args.uri); got != tt.want {
				t.Errorf("matchNoCacheRule() = %v, want %v", got, tt.want)
			}
		})
	}
}
