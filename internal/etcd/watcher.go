package etcd

import (
	"time"

	"go.etcd.io/etcd/client"
)

// Connect to etcd
func Connect(addrs ...string) (client.KeysAPI, error) {
	cfg := client.Config{
		Endpoints:               addrs,
		Transport:               client.DefaultTransport,
		HeaderTimeoutPerRequest: 3 * time.Second, // set timeout per request to fail fast when the target endpoint is unavailable
	}

	c, err := client.New(cfg)
	if err != nil {
		return nil, err
	}

	return client.NewKeysAPI(c), nil
}
