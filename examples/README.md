# demo of `api-proxier`

### Step 1: Prepare data

prepare 2 server instance of cluster(ID=1), also prepare 1 routing rules and 3 API rules like:

2 server instance
```go
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
```

1 routing rules
```go
routing1 := &models.Routing{
Idx:             "1",
Prefix:          "/srv1",
ClusterID:       "1",
NeedStripPrefix: true,
}
store.Set("/routings/1", encode(routing1), -1)
```

3 API rules
```go
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
```

exec it
```sh
go run prepare.go
```

### Step 2: run servers
```sh
go run servers.go
```

### Step 3: start `api-proxier`

```sh
./api-proxier -etcd-addr=http://127.0.0.1:2379 -port=9000
```

### Step 4: Requets api

```sh
curl -X GET http://127.0.0.1/examples/combination
```

get result:

```sh
{
  "code": 0,
  "id": {
    "code": 0,
    "data": "error, empty id"
  },
  "message": "combine result",
  "name": {
    "code": 0,
    "data": "server2"
  }
}
```