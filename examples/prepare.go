package main

import (
	"encoding/json"
	"net/http"

	// "github.com/jademperor/common/configs"
	"github.com/jademperor/common/etcdutils"
	"github.com/jademperor/common/models"
)

func encode(v interface{}) string {
	byts, _ := json.Marshal(v)
	return string(byts)
}

func prepareServerInstance(store etcdutils.Store) {
	srvIns1 := &models.ServerInstance{
		Idx:       "1",
		Name:      "server1",
		Addr:      "http://localhost:9091",
		Weight:    5,
		ClusterID: "1",
	}
	store.Set("/clusters/1/1", encode(srvIns1), -1)

	srvIns2 := &models.ServerInstance{
		Idx:       "2",
		Name:      "server2",
		Addr:      "http://localhost:9092",
		Weight:    5,
		ClusterID: "1",
	}
	store.Set("/clusters/1/2", encode(srvIns2), -1)
}

func prepareRouting(store etcdutils.Store) {
	routing1 := &models.Routing{
		Idx:             "1",
		Prefix:          "/srv1",
		ClusterID:       "1",
		NeedStripPrefix: true,
	}
	store.Set("/routings/1", encode(routing1), -1)
}

func prepareAPIs(store etcdutils.Store) {
	api1 := &models.API{
		Idx:             "1",
		Path:            "/example/name",
		Method:          http.MethodGet,
		TargetClusterID: "1",
		RewritePath:     "/srv/name",
		NeedCombine:     false,
		CombineReqCfgs:  nil,
	}
	store.Set("/apis/1", encode(api1), -1)

	api2 := &models.API{
		Idx:             "2",
		Path:            "/example/id",
		Method:          http.MethodGet,
		TargetClusterID: "1",
		RewritePath:     "srv/id",
		NeedCombine:     false,
		CombineReqCfgs:  nil,
	}
	store.Set("/apis/2", encode(api2), -1)

	api3 := &models.API{
		Idx:             "3",
		Path:            "/example/combination",
		Method:          http.MethodGet,
		TargetClusterID: "",
		RewritePath:     "",
		NeedCombine:     true,
		CombineReqCfgs: []*models.APICombination{
			&models.APICombination{
				Idx:             "1",
				Path:            "/srv/id",
				Field:           "id",
				Method:          http.MethodGet,
				TargetClusterID: "1",
			},
			&models.APICombination{
				Idx:             "2",
				Path:            "/srv/name",
				Field:           "name",
				Method:          http.MethodGet,
				TargetClusterID: "1",
			},
		},
	}
	store.Set("/apis/3", encode(api3), -1)

}

func main() {
	addrs := []string{"http://127.0.0.1:2377", "http://127.0.0.1:2378", "http://127.0.0.1:2379"}
	store, err := etcdutils.NewEtcdStore(addrs)
	if err != nil {
		panic(err)
	}

	prepareServerInstance(store)
	prepareRouting(store)
	prepareAPIs(store)
}
