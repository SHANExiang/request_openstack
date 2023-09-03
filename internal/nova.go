package internal

import (
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"log"
	"request_openstack/consts"
	"request_openstack/internal/entity"
	"request_openstack/utils"
	"strconv"
	"sync"
	"time"
)

var supportedNovaResourceTypes = [...]string{
	consts.SERVER,
}

type Nova struct {
	Request
	ProjectId             string
	headers               map[string]string
	DB                    *gorm.DB
	DeleteChannels        map[string]chan Output
	mu                    sync.Mutex
	snowflake             *utils.Snowflake
}

func initNovaOutputChannels() map[string]chan Output {
	outputChannel := make(map[string]chan Output)
	for _, resourceType := range supportedNovaResourceTypes {
		outputChannel[resourceType] = make(chan Output, 0)
	}
	return outputChannel
}

func NewNova(options ...Option) *Nova {
	opts := new(Options)
	for _, option := range options {
		option(opts)
	}
	var nova *Nova
	nova = &Nova{
		DeleteChannels: initNovaOutputChannels(),
	}
	nova.ProjectId = opts.ProjectId
	headers := make(map[string]string)
	headers[consts.AuthToken] = opts.Token
	nova.headers = headers
	nova.Request = opts.Request
	nova.snowflake = opts.Snowflake
	return nova
}

func (n *Nova) getDeleteChannel(resourceType string) chan Output {
	defer n.mu.Unlock()
	n.mu.Lock()
	return n.DeleteChannels[resourceType]
}

func (n *Nova) makeDeleteChannel(resourceType string, length int) chan Output {
	defer n.mu.Unlock()
	n.mu.Lock()
	n.DeleteChannels[resourceType] = make(chan Output, length)
	return n.DeleteChannels[resourceType]
}

// GetPciDevicesByHost 获取GPU等外接设备
func (n *Nova) GetPciDevicesByHost()  {
	resp := n.Get(n.headers, "/os-pci-devices?node_name=com04.vim1.local&host_name=com04.vim1.local")
	fmt.Println(resp)
}


// GetComputeServices 获取计算服务
func (n *Nova) GetComputeServices() entity.ComputeServices {
	resp := n.Get(n.headers, "/os-services")
	var services entity.ComputeServices
	_ = json.Unmarshal(resp, &services)
	return services
}

func (n *Nova) GetInstanceDetail(instanceId string) (server *entity.ServerMap) {
	resp := n.Get(n.headers, fmt.Sprintf("servers/%s", instanceId))
	if resp != nil {
		_ = json.Unmarshal(resp, &server)

		//cache.RedisClient.SetMap(n.tag + consts.INSTANCES, server.Server.Id, server)
	}
	return server
}

func (n *Nova) makeSureInstanceActive(instanceId string) {
	instance := n.GetInstanceDetail(instanceId)
	if instance == nil {
		time.Sleep(2 * time.Second)
		for instance == nil {
			instance = n.GetInstanceDetail(instanceId)
			time.Sleep(2 * time.Second)
		}
	}
	timeout := 2 * 60 * time.Second
	done := make(chan bool, 1)
	go func() {
		state := instance.Server.Status
		for state != "ACTIVE" {
			time.Sleep(10 * time.Second)
			instance = n.GetInstanceDetail(instanceId)
			state =instance.Server.Status
		}
		done <- true
	}()
	select {
	case <-done:
		log.Println("*******************Create instance success")
	case <-time.After(timeout):
		log.Println("*******************Create instance timeout")
	}
}

func (n *Nova) CreateInstance(opts *entity.CreateInstanceOpts) string {
	urlSuffix := "servers"
	opts.Name = fmt.Sprintf("%s_%s", opts.Name + strconv.FormatUint(n.snowflake.NextVal(), 10), consts.SERVER)
	opts.UserData = "Q29udGVudC1UeXBlOiBtdWx0aXBhcnQvbWl4ZWQ7IGJvdW5kYXJ5PSI9PT09PT09PT09PT09PT0yMzA5OTg0MDU5NzQzNzYyNDc1PT0iIApNSU1FLVZlcnNpb246IDEuMAoKLS09PT09PT09PT09PT09PT0yMzA5OTg0MDU5NzQzNzYyNDc1PT0KQ29udGVudC1UeXBlOiB0ZXh0L2Nsb3VkLWNvbmZpZzsgY2hhcnNldD0idXMtYXNjaWkiIApNSU1FLVZlcnNpb246IDEuMApDb250ZW50LVRyYW5zZmVyLUVuY29kaW5nOiA3Yml0CkNvbnRlbnQtRGlzcG9zaXRpb246IGF0dGFjaG1lbnQ7IGZpbGVuYW1lPSJzc2gtcHdhdXRoLXNjcmlwdC50eHQiIAoKI2Nsb3VkLWNvbmZpZwpkaXNhYmxlX3Jvb3Q6IGZhbHNlCnNzaF9wd2F1dGg6IHRydWUKcGFzc3dvcmQ6IFdhbmcuMTIzCgotLT09PT09PT09PT09PT09PTIzMDk5ODQwNTk3NDM3NjI0NzU9PQpDb250ZW50LVR5cGU6IHRleHQveC1zaGVsbHNjcmlwdDsgY2hhcnNldD0idXMtYXNjaWkiIApNSU1FLVZlcnNpb246IDEuMApDb250ZW50LVRyYW5zZmVyLUVuY29kaW5nOiA3Yml0CkNvbnRlbnQtRGlzcG9zaXRpb246IGF0dGFjaG1lbnQ7IGZpbGVuYW1lPSJwYXNzd2Qtc2NyaXB0LnR4dCIgCgojIS9iaW4vc2gKZWNobyAncm9vdDpXYW5nLjEyMycgfCBjaHBhc3N3ZAoKLS09PT09PT09PT09PT09PT0yMzA5OTg0MDU5NzQzNzYyNDc1PT0KQ29udGVudC1UeXBlOiB0ZXh0L3gtc2hlbGxzY3JpcHQ7IGNoYXJzZXQ9InVzLWFzY2lpIiAKTUlNRS1WZXJzaW9uOiAxLjAKQ29udGVudC1UcmFuc2Zlci1FbmNvZGluZzogN2JpdApDb250ZW50LURpc3Bvc2l0aW9uOiBhdHRhY2htZW50OyBmaWxlbmFtZT0iZW5hYmxlLWZzLWNvbGxlY3Rvci50eHQiIAoKIyEvYmluL3NoCnFlbXVfZmlsZT0iL2V0Yy9zeXNjb25maWcvcWVtdS1nYSIKaWYgWyAtZiAke3FlbXVfZmlsZX0gXTsgdGhlbgogICAgc2VkIC1pIC1yICJzL14jP0JMQUNLTElTVF9SUEM9LyNCTEFDS0xJU1RfUlBDPS8iICIke3FlbXVfZmlsZX0iCiAgICBoYXNfZ3FhPSQoc3lzdGVtY3RsIGxpc3QtdW5pdHMgLS1mdWxsIC1hbGwgLXQgc2VydmljZSAtLXBsYWluIHwgZ3JlcCAtbyBxZW11LWd1ZXN0LWFnZW50LnNlcnZpY2UpCiAgICBpZiBbWyAtbiAke2hhc19ncWF9IF1dOyB0aGVuCiAgICAgICAgc3lzdGVtY3RsIHJlc3RhcnQgcWVtdS1ndWVzdC1hZ2VudC5zZXJ2aWNlCiAgICBmaQpmaQoKLS09PT09PT09PT09PT09PT0yMzA5OTg0MDU5NzQzNzYyNDc1PT0tLQ=="
    reqBody := opts.ToRequestBody()
    log.Printf("headers==%+v\n", n.headers)
    resp := n.Post(n.headers, urlSuffix, reqBody)
    var server entity.ServerMap
    _ = json.Unmarshal(resp, &server)
	instanceId := server.Server.Id

    //cache.RedisClient.SetMap(n.tag + consts.INSTANCES, instanceId, server)
	log.Println("==============Create instance success", instanceId)
	n.makeSureInstanceActive(instanceId)
    return instanceId
}

//func (n *Nova) constructPostBody(imageRef, flavorRef, networkId string) string {
//    uuid := n.snowflake.NextVal()
//    reqBody := fmt.Sprintf("{\"server\": {\"name\": \"dxtest_%d\", \"imageRef\": \"%+v\", \"flavorRef\": \"%+v\", \"networks\": [{\"uuid\": \"%+v\"}], \"user_data\": \"Q29udGVudC1UeXBlOiBtdWx0aXBhcnQvbWl4ZWQ7IGJvdW5kYXJ5PSI9PT09PT09PT09PT09PT0yMzA5OTg0MDU5NzQzNzYyNDc1PT0iIApNSU1FLVZlcnNpb246IDEuMAoKLS09PT09PT09PT09PT09PT0yMzA5OTg0MDU5NzQzNzYyNDc1PT0KQ29udGVudC1UeXBlOiB0ZXh0L2Nsb3VkLWNvbmZpZzsgY2hhcnNldD0idXMtYXNjaWkiIApNSU1FLVZlcnNpb246IDEuMApDb250ZW50LVRyYW5zZmVyLUVuY29kaW5nOiA3Yml0CkNvbnRlbnQtRGlzcG9zaXRpb246IGF0dGFjaG1lbnQ7IGZpbGVuYW1lPSJzc2gtcHdhdXRoLXNjcmlwdC50eHQiIAoKI2Nsb3VkLWNvbmZpZwpkaXNhYmxlX3Jvb3Q6IGZhbHNlCnNzaF9wd2F1dGg6IHRydWUKcGFzc3dvcmQ6IFdhbmcuMTIzCgotLT09PT09PT09PT09PT09PTIzMDk5ODQwNTk3NDM3NjI0NzU9PQpDb250ZW50LVR5cGU6IHRleHQveC1zaGVsbHNjcmlwdDsgY2hhcnNldD0idXMtYXNjaWkiIApNSU1FLVZlcnNpb246IDEuMApDb250ZW50LVRyYW5zZmVyLUVuY29kaW5nOiA3Yml0CkNvbnRlbnQtRGlzcG9zaXRpb246IGF0dGFjaG1lbnQ7IGZpbGVuYW1lPSJwYXNzd2Qtc2NyaXB0LnR4dCIgCgojIS9iaW4vc2gKZWNobyAncm9vdDpXYW5nLjEyMycgfCBjaHBhc3N3ZAoKLS09PT09PT09PT09PT09PT0yMzA5OTg0MDU5NzQzNzYyNDc1PT0KQ29udGVudC1UeXBlOiB0ZXh0L3gtc2hlbGxzY3JpcHQ7IGNoYXJzZXQ9InVzLWFzY2lpIiAKTUlNRS1WZXJzaW9uOiAxLjAKQ29udGVudC1UcmFuc2Zlci1FbmNvZGluZzogN2JpdApDb250ZW50LURpc3Bvc2l0aW9uOiBhdHRhY2htZW50OyBmaWxlbmFtZT0iZW5hYmxlLWZzLWNvbGxlY3Rvci50eHQiIAoKIyEvYmluL3NoCnFlbXVfZmlsZT0iL2V0Yy9zeXNjb25maWcvcWVtdS1nYSIKaWYgWyAtZiAke3FlbXVfZmlsZX0gXTsgdGhlbgogICAgc2VkIC1pIC1yICJzL14jP0JMQUNLTElTVF9SUEM9LyNCTEFDS0xJU1RfUlBDPS8iICIke3FlbXVfZmlsZX0iCiAgICBoYXNfZ3FhPSQoc3lzdGVtY3RsIGxpc3QtdW5pdHMgLS1mdWxsIC1hbGwgLXQgc2VydmljZSAtLXBsYWluIHwgZ3JlcCAtbyBxZW11LWd1ZXN0LWFnZW50LnNlcnZpY2UpCiAgICBpZiBbWyAtbiAke2hhc19ncWF9IF1dOyB0aGVuCiAgICAgICAgc3lzdGVtY3RsIHJlc3RhcnQgcWVtdS1ndWVzdC1hZ2VudC5zZXJ2aWNlCiAgICBmaQpmaQoKLS09PT09PT09PT09PT09PT0yMzA5OTg0MDU5NzQzNzYyNDc1PT0tLQ==\", \"adminPass\": \"Wang.123\",  \"security_groups\": [{\"name\": \"sdn_test\"}]}}", uuid, imageRef, flavorRef, networkId)
//    return reqBody
//}

func (n *Nova) listInstancesByProject() entity.Servers {
	urlSuffix := fmt.Sprintf("servers/detail?all_tenants=True&tenant_id=%s", n.ProjectId)
    resp := n.List(n.headers, urlSuffix)
    var instances entity.Servers
    _ = json.Unmarshal(resp, &instances)
	log.Println("==============List instance success, there had", len(instances.Servers))
    return instances
}

func (n *Nova) DeleteInstance(instanceId string) Output {
	outputObj := Output{ParametersMap: map[string]string{"instance_id": instanceId}}
	defer func() {
		if err := recover(); err != nil {
			log.Println("catch error：", err)
			outputObj.Success = false
			outputObj.Response = err
		}
	}()

	urlSuffix := fmt.Sprintf("servers/%s/action", instanceId)
	forceDelete := `{"forceDelete": null}`
	n.Post(n.headers, urlSuffix, forceDelete)
	instance := n.GetInstanceDetail(instanceId)
	for instance != nil {
		time.Sleep(5 * time.Second)
		instance = n.GetInstanceDetail(instanceId)
	}
	outputObj.Success, outputObj.Response = true, ""
	return outputObj
}

func (n *Nova) DeleteServers() {
	instances := n.listInstancesByProject()
	ch := n.makeDeleteChannel(consts.SERVER, len(instances.Servers))
	for _, instance := range instances.Servers {
		tempInstance := instance
		go func() {
			ch <- n.DeleteInstance(tempInstance.Id)
		}()
	}
	if len(ch) != cap(ch) {
		for len(ch) != cap(ch) {}
	}
	log.Println("Instances were deleted completely")
}

func (n *Nova) CreateMultipleInstances(opts entity.CreateInstanceOpts) {
	urlSuffix := "servers"
	opts.Name = fmt.Sprintf("%s_%s", opts.Name + strconv.FormatUint(n.snowflake.NextVal(), 10), consts.SERVER)
	opts.UserData = "Q29udGVudC1UeXBlOiBtdWx0aXBhcnQvbWl4ZWQ7IGJvdW5kYXJ5PSI9PT09PT09PT09PT09PT0yMzA5OTg0MDU5NzQzNzYyNDc1PT0iIApNSU1FLVZlcnNpb246IDEuMAoKLS09PT09PT09PT09PT09PT0yMzA5OTg0MDU5NzQzNzYyNDc1PT0KQ29udGVudC1UeXBlOiB0ZXh0L2Nsb3VkLWNvbmZpZzsgY2hhcnNldD0idXMtYXNjaWkiIApNSU1FLVZlcnNpb246IDEuMApDb250ZW50LVRyYW5zZmVyLUVuY29kaW5nOiA3Yml0CkNvbnRlbnQtRGlzcG9zaXRpb246IGF0dGFjaG1lbnQ7IGZpbGVuYW1lPSJzc2gtcHdhdXRoLXNjcmlwdC50eHQiIAoKI2Nsb3VkLWNvbmZpZwpkaXNhYmxlX3Jvb3Q6IGZhbHNlCnNzaF9wd2F1dGg6IHRydWUKcGFzc3dvcmQ6IFdhbmcuMTIzCgotLT09PT09PT09PT09PT09PTIzMDk5ODQwNTk3NDM3NjI0NzU9PQpDb250ZW50LVR5cGU6IHRleHQveC1zaGVsbHNjcmlwdDsgY2hhcnNldD0idXMtYXNjaWkiIApNSU1FLVZlcnNpb246IDEuMApDb250ZW50LVRyYW5zZmVyLUVuY29kaW5nOiA3Yml0CkNvbnRlbnQtRGlzcG9zaXRpb246IGF0dGFjaG1lbnQ7IGZpbGVuYW1lPSJwYXNzd2Qtc2NyaXB0LnR4dCIgCgojIS9iaW4vc2gKZWNobyAncm9vdDpXYW5nLjEyMycgfCBjaHBhc3N3ZAoKLS09PT09PT09PT09PT09PT0yMzA5OTg0MDU5NzQzNzYyNDc1PT0KQ29udGVudC1UeXBlOiB0ZXh0L3gtc2hlbGxzY3JpcHQ7IGNoYXJzZXQ9InVzLWFzY2lpIiAKTUlNRS1WZXJzaW9uOiAxLjAKQ29udGVudC1UcmFuc2Zlci1FbmNvZGluZzogN2JpdApDb250ZW50LURpc3Bvc2l0aW9uOiBhdHRhY2htZW50OyBmaWxlbmFtZT0iZW5hYmxlLWZzLWNvbGxlY3Rvci50eHQiIAoKIyEvYmluL3NoCnFlbXVfZmlsZT0iL2V0Yy9zeXNjb25maWcvcWVtdS1nYSIKaWYgWyAtZiAke3FlbXVfZmlsZX0gXTsgdGhlbgogICAgc2VkIC1pIC1yICJzL14jP0JMQUNLTElTVF9SUEM9LyNCTEFDS0xJU1RfUlBDPS8iICIke3FlbXVfZmlsZX0iCiAgICBoYXNfZ3FhPSQoc3lzdGVtY3RsIGxpc3QtdW5pdHMgLS1mdWxsIC1hbGwgLXQgc2VydmljZSAtLXBsYWluIHwgZ3JlcCAtbyBxZW11LWd1ZXN0LWFnZW50LnNlcnZpY2UpCiAgICBpZiBbWyAtbiAke2hhc19ncWF9IF1dOyB0aGVuCiAgICAgICAgc3lzdGVtY3RsIHJlc3RhcnQgcWVtdS1ndWVzdC1hZ2VudC5zZXJ2aWNlCiAgICBmaQpmaQoKLS09PT09PT09PT09PT09PT0yMzA5OTg0MDU5NzQzNzYyNDc1PT0tLQ=="
    reqBody := opts.ToRequestBody()
    n.headers["OpenStack-API-Version"] = "compute 2.74"
    resp := n.Post(n.headers, urlSuffix, reqBody)

    log.Println(string(resp))
}

func (n *Nova) GetComputeHosts() []string {
    services := n.GetComputeServices()
    var hosts []string
    for _, service := range services.Services {
    	if service.Binary == "nova-compute" {
    		hosts = append(hosts, service.Host)
		}
	}
	return hosts
}

func (n *Nova) CreateAggregate(opts entity.CreateAggregateOpts) int {
	urlSuffix := "os-aggregates"
    reqBody := opts.ToRequestBody()
    resp := n.Post(n.headers, urlSuffix, reqBody)
    var aggregate entity.AggregateMap
    _ = json.Unmarshal(resp, &aggregate)
	log.Println("==============Create aggregate success", aggregate.Aggregate.Id)
    return aggregate.Aggregate.Id
}

func (n *Nova) AggregateAddHost(aggregateId int, opts entity.AddHostOpts) int {
	urlSuffix := fmt.Sprintf("os-aggregates/%d/action", aggregateId)
	reqBody := opts.ToRequestBody()
	resp := n.Post(n.headers, urlSuffix, reqBody)
	var aggregate entity.AggregateMap
	_ = json.Unmarshal(resp, &aggregate)
	log.Println("==============Aggregate add host success", aggregate.Aggregate.Id)
	return aggregate.Aggregate.Id
}

func (n *Nova) AggregateSetMetadata(aggregateId int, opts entity.SetMetadataOpts) int {
	urlSuffix := fmt.Sprintf("os-aggregates/%d/action", aggregateId)
	reqBody := opts.ToRequestBody()
	resp := n.Post(n.headers, urlSuffix, reqBody)
	var aggregate entity.AggregateMap
	_ = json.Unmarshal(resp, &aggregate)
	log.Println("==============Aggregate set metadata success", aggregate.Aggregate.Id)
	return aggregate.Aggregate.Id
}

// CreateConsole create a new console api
func (n *Nova) CreateConsole(serverId string, opts entity.CreateConsoleOpts) entity.RemoteConsoleMap {
    urlSuffix := fmt.Sprintf("servers/%s/remote-consoles", serverId)
    reqBody := opts.ToRequestBody()
    resp := n.Post(n.headers, urlSuffix, reqBody)

    var console entity.RemoteConsoleMap
    _ = json.Unmarshal(resp, &console)
	log.Println("==============Create console success", serverId)
    return console
}

// AttachVolume attach volume
func (n *Nova) AttachVolume(instanceId, volumeId string) {
	urlSuffix := fmt.Sprintf("servers/%s/os-volume_attachments", instanceId)
	formatter := `{
		"volumeAttachment": {
			"volumeId": "%+v"
		}
	}`
	reqBody := fmt.Sprintf(formatter, volumeId)
	resp := n.Post(n.headers, urlSuffix, reqBody)

	log.Println("==============Attach Volume success", string(resp))
}

// DetachVolume detach volume
func (n *Nova) DetachVolume(instanceId, volumeId string) {
	urlSuffix := fmt.Sprintf("servers/%s/os-volume_attachments/%s", instanceId, volumeId)
	if ok, _ := n.Delete(n.headers, urlSuffix); ok {
		log.Println("==============Detach volume success", instanceId)
		return
	}
	log.Println("==============Detach volume failed", instanceId)
}

func (n *Nova) CreateFlavorExtraSpecs(flavorId string) {
    urlSuffix := fmt.Sprintf("flavors/%s/os-extra_specs", flavorId)
    formatter := `{
                     "extra_specs": {
                         "pci_passthrough:alias": "a4:1"
                     }
                  }`
    resp := n.Post(n.headers, urlSuffix, formatter)
    log.Println(string(resp))
	log.Println("==============Create extra specs success", flavorId)
}

func (n *Nova) GetDBBDM(instanceId string) {

}