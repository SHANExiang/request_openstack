package manager

import (
    "fmt"
    "log"
    "request_openstack/configs"
    "sync"
)

type Transmitter struct {
    Type               string
    Data               map[string]string
}

type Scheduler struct {
    Manager                 *Manager
    completedOuts           sync.Map
    nodes                   map[string]*Node
    completedChannel        chan struct{}
}

func NewScheduler(yamlFile string) *Scheduler {
    resources := yamlToMap(yamlFile)
    ResourcesMap = Resolve(resources)
    adminManager := NewManager()
    nodes := nodeAssociateResources(InitNodes())
    scheduler := &Scheduler{
        Manager: adminManager,
        nodes: nodes,
        completedChannel: make(chan struct{}, len(nodes)),
    }
    adminManager.EnsureSgExist(configs.CONF.ProjectName)
    return scheduler
}

func (s *Scheduler) waitDepsThenCall(resource string, wg *sync.WaitGroup) {
    var trans Transmitter
    trans.Data = make(map[string]string)
    for dep, fieldName := range ResourcesMap[resource].Dependencies {
        out, ok := s.completedOuts.Load(dep)
        for !ok {
            out, ok = s.completedOuts.Load(dep)
        }
        if out.(Output).IsSuccess {
            trans.Type = ResourcesMap[dep].Type
            trans.Data[fieldName] = out.(Output).Resp
        } else {
            errorInfo := fmt.Sprintf("The dependency %s of resource %s call failed", dep, resource)
            s.completedOuts.Store(dep, Output{Type: resource, IsSuccess: false, Resp: errorInfo})
            return
        }
    }
    ResourcesMap[resource].Create(s.Manager, &s.completedOuts, &trans, wg)
}

func (s *Scheduler) notify(node *Node) {
    if len(node.monitorCreateChannel) != cap(node.monitorCreateChannel) {
        for len(node.monitorCreateChannel) != cap(node.monitorCreateChannel) {}
    }
    var wg sync.WaitGroup
    for _, resourceName := range node.resources {
        wg.Add(1)
        if len(ResourcesMap[resourceName].Dependencies) == 0 {
            go ResourcesMap[resourceName].Create(s.Manager, &s.completedOuts, nil, &wg)
        } else {
            go s.waitDepsThenCall(resourceName, &wg)
        }
    }
    wg.Wait()
    for _, dep := range node.requiredBy {
        s.nodes[dep.resourceType].monitorCreateChannel <- struct{}{}
        log.Printf("%s completed, notify %s", node.resourceType, dep.resourceType)
    }
    s.completedChannel <- struct{}{}
    log.Printf("%s call completed", node.resourceType)
}

func (s *Scheduler) call() {
    for _, node := range s.nodes {
        go s.notify(node)
    }
    if len(s.completedChannel) != cap(s.completedChannel) {
        for len(s.completedChannel) != cap(s.completedChannel) {}
    }
    log.Println("##############Completed##############")
}

func (s *Scheduler) Run() {
    s.call()
}