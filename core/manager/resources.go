package manager

import (
    "fmt"
    "log"
    "reflect"
    "request_openstack/consts"
    "request_openstack/internal/entity"
    "strings"
    "sync"
)

var (
	ResourcesMap      = make(map[string]Resource)
	RW                = sync.RWMutex{}

    netDoneChannel    chan struct{}
)

type Resource struct {
    Name               string
    Type               string
    PropsObj           interface{}
    Dependencies       map[string]string
    Done               bool
}

type Output struct {
    Type                  string
    IsSuccess             bool
    Resp                  string
}

func NewResource(name, typ string, propsObj interface{}, deps map[string]string) Resource {
    return Resource{
        Name: name,
        Type: typ,
        PropsObj: propsObj,
        Dependencies: deps,
        Done: false,
    }
}

func (r Resource) Create(manager *Manager, completedOuts *sync.Map, trans *Transmitter, wg *sync.WaitGroup) {
    log.Println("##############Creating", r.Name, r.Type)
    var out Output
    defer func() {
        if err := recover(); err != nil {
            out.Type = r.Name
            out.IsSuccess = false
            out.Resp = fmt.Sprintf("panic err %s", err)
        }
        completedOuts.Store(r.Name, out)
        wg.Done()
        log.Println(fmt.Sprintf("##############Create resource " +
            "%s resp ---> %s", out.Type, out.Resp))
    }()
    out.Type = r.Name
    out.IsSuccess = true
    switch r.Type {
    case consts.NETWORK:
        opts := r.PropsObj.(*entity.CreateNetworkOpts)
        out.Resp = manager.CreateNetwork(opts)
    case consts.SUBNET:
        opts := r.PropsObj.(*entity.CreateSubnetOpts)
        if trans != nil {
            for key, value := range trans.Data {
                reflect.ValueOf(opts).Elem().FieldByName(key).SetString(value)
            }
        }
        out.Resp = manager.CreateSubnet(opts)
    case consts.ROUTER:
        opts := r.PropsObj.(*entity.CreateRouterOpts)
        out.Resp = manager.CreateRouter(opts)
    case consts.ROUTERINTERFACE:
        opts := r.PropsObj.(*entity.AddRouterInterfaceOpts)
        if trans != nil {
            for key, value := range trans.Data {
                reflect.ValueOf(opts).Elem().FieldByName(key).SetString(value)
            }
        }
        out.Resp = manager.AddRouterInterface(opts)
    case consts.FLOATINGIP:
        opts := r.PropsObj.(*entity.CreateFipOpts)
        if trans != nil {
            for key, value := range trans.Data {
                if trans.Type == consts.SERVER {
                    value, _ = manager.GetInstancePort(value)
                }
                reflect.ValueOf(opts).Elem().FieldByName(key).SetString(value)
            }
        }
        out.Resp = manager.CreateFloatingIP(opts)
    case consts.SERVER:
        opts := r.PropsObj.(*entity.CreateInstanceOpts)
        if trans != nil {
            for key, value := range trans.Data {
                if strings.Contains(key, "/") {
                    field := strings.Split(key, "/")
                    firstField := field[0]
                    if firstField == "Networks" {
                        objSlice := make([]entity.ServerNet, 0)
                        obj := entity.ServerNet{}
                        reflect.ValueOf(&obj).Elem().FieldByName(field[1]).SetString(value)
                        objSlice = append(objSlice, obj)
                        reflect.ValueOf(opts).Elem().FieldByName(firstField).Set(reflect.ValueOf(objSlice))
                    }
                }
            }
        }
        out.Resp = manager.CreateInstance(opts)
    }
}
