package manager

import (
    "gopkg.in/yaml.v3"
    "io/ioutil"
    "log"
    "request_openstack/consts"
    "request_openstack/internal/entity"
)


func yamlToMap(yamlFile string) map[string]interface{} {
    data, err := ioutil.ReadFile(yamlFile)
    if err != nil {
        log.Println("##############Read file failed")
        return nil
    }
    var resources map[string]interface{}
    err = yaml.Unmarshal(data, &resources)
    if err != nil {
        log.Println("##############Resolve yaml failed")
        return nil
    }
    return resources
}

func Resolve(resources map[string]interface{}) map[string]Resource {
    var resourceMap = make(map[string]Resource)
    for key, resource := range resources["resources"].(map[string]interface{}) {
        resourceType := resource.(map[string]interface{})["type"].(string)
        var resourceProps map[string]interface{}
        if val, ok := resource.(map[string]interface{})["properties"]; ok {
            resourceProps = val.(map[string]interface{})
        }
        if resourceType == consts.NETWORK {
            optsObj := &entity.CreateNetworkOpts{Name: key}
            optsObj, dependencies := optsObj.AssignProps(resourceProps)
            resourceMap[key] = NewResource(key, resourceType, optsObj, dependencies)
        } else if resourceType == consts.SUBNET {
            optsObj := &entity.CreateSubnetOpts{Name: key}
            optsObj, dependencies := optsObj.AssignProps(resourceProps)
            resourceMap[key] = NewResource(key, resourceType, optsObj, dependencies)
        } else if resourceType == consts.ROUTER {
            optsObj := &entity.CreateRouterOpts{Name: key}
            optsObj, dependencies := optsObj.AssignProps(resourceProps)
            resourceMap[key] = NewResource(key, resourceType, optsObj, dependencies)
        } else if resourceType == consts.ROUTERINTERFACE {
            optsObj := &entity.AddRouterInterfaceOpts{}
            optsObj, dependencies := optsObj.AssignProps(resourceProps)
            resourceMap[key] = NewResource(key, resourceType, optsObj, dependencies)
        } else if resourceType == consts.FLOATINGIP {
            optsObj := &entity.CreateFipOpts{}
            optsObj, dependencies := optsObj.AssignProps(resourceProps)
            resourceMap[key] = NewResource(key, resourceType, optsObj, dependencies)
        } else if resourceType == consts.SERVER {
            optsObj := &entity.CreateInstanceOpts{}
            optsObj, dependencies := optsObj.AssignProps(resourceProps)
            resourceMap[key] = NewResource(key, resourceType, optsObj, dependencies)
        }
    }
    return resourceMap
}
