package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"request_openstack/consts"
	"request_openstack/internal/cache"
	"request_openstack/internal/entity"
	"request_openstack/utils"
	"strconv"
	"sync"
	"time"
)

var supportedNeutronResourceTypes = [...]string{
	consts.NETWORK, consts.SUBNET, consts.PORT, consts.SECURITYGROUPRULE, consts.SECURITYGROUP,
	consts.BANDWIDTH_LIMIT_RULE, consts.DSCP_MARKING_RULE, consts.MINIMUM_BANDWIDTH_RULE,
	consts.QOS_POLICY, consts.ROUTER, consts.ROUTERINTERFACE, consts.ROUTERGATEWAY,
	consts.ROUTERROUTE, consts.FLOATINGIP, consts.PORTFORWARDING, consts.FIREWALLRULE,
	consts.FIREWALLPOLICY, consts.FIREWALL, consts.VpcConnection,
}

type Neutron struct {
	Request
	projectId          string
	headers            map[string]string
	wg                 *sync.WaitGroup
	DeleteChannels     map[string]chan Output
	mu                 sync.Mutex
	snowflake          *utils.Snowflake
	isAdmin            bool
}

func initNeutronOutputChannels() map[string]chan Output {
	outputChannel := make(map[string]chan Output)
	for _, resourceType := range supportedNeutronResourceTypes {
		outputChannel[resourceType] = make(chan Output, 0)
	}
	return outputChannel
}

func NewNeutron(options ...Option) *Neutron {
	opts := new(Options)
	for _, option := range options {
		option(opts)
	}
    var neutron *Neutron
	neutron = &Neutron{
		wg: &sync.WaitGroup{},
		DeleteChannels: initNeutronOutputChannels(),
		Request: opts.Request,
	}
	neutron.projectId = opts.ProjectId
	headers := make(map[string]string)
	headers[consts.AuthToken] = opts.Token
	neutron.headers = headers
	neutron.snowflake = opts.Snowflake
	neutron.isAdmin = opts.IsAdmin
	return neutron
}


type getterFunc func(resourceId string) map[string]interface{}

func (g getterFunc) get(resourceId string) map[string]interface{} {
	return g(resourceId)
}

func (n *Neutron) getDeleteChannel(resourceType string) chan Output {
	defer n.mu.Unlock()
	n.mu.Lock()
	return n.DeleteChannels[resourceType]
}

func (n *Neutron) MakeDeleteChannel(resourceType string, length int) chan Output {
	defer n.mu.Unlock()
	n.mu.Lock()

	n.DeleteChannels[resourceType] = make(chan Output, length)
	return n.DeleteChannels[resourceType]
}

// network

func (n *Neutron) CreateNetwork(opts *entity.CreateNetworkOpts) string {
	opts.Name = fmt.Sprintf("%s_%s", opts.Name + strconv.FormatUint(n.snowflake.NextVal(), 10), consts.NETWORK)
	reqBody := opts.ToRequestBody()
	resp := n.Post(n.headers, consts.NETWORKS, reqBody)

	var network entity.NetworkMap
	_ = json.Unmarshal(resp, &network)

	//cache.RedisClient.SetMap(n.tag + consts.NETWORKS, network.Network.Id, network)
	log.Println("==============Create internal network success", network.Network.Id)
	return network.Network.Id
}

func (n *Neutron) GetNetwork(networkId string) entity.NetworkMap {
	urlSuffix := fmt.Sprintf("networks/%s", networkId)
	resp := n.Get(n.headers, urlSuffix)
	var obj entity.NetworkMap
	_ = json.Unmarshal(resp, &obj)
	return obj
}

func (n *Neutron) UpdateNetwork(netId string, updateBody string) {
	urlSuffix := fmt.Sprintf("networks/%s", netId)
	resp := n.Put(n.headers, urlSuffix, updateBody)

	var net entity.NetworkMap
    _ = json.Unmarshal(resp, &net)
	//n.SyncMap(n.tag + consts.NETWORKS, netId, nil, net)
	log.Printf("==============Update network %s success\n", netId)
}

func (n *Neutron) UpdateNetWithQos(netId, qosId string) string {
	updateBody := fmt.Sprintf("{\"network\": {\"qos_policy_id\": \"%+v\"}}", qosId)
    n.UpdateNetwork(netId, updateBody)
	return netId
}

func (n *Neutron) ListNetworks() entity.Networks {
	var urlSuffix string
	if n.isAdmin {
		urlSuffix = consts.NETWORKS
	} else {
		urlSuffix = fmt.Sprintf("networks?project_id=%s", n.projectId)
	}
	//urlSuffix := "networks"
	resp := n.List(n.headers, urlSuffix)
	var networks entity.Networks
	_ = json.Unmarshal(resp, &networks)
	log.Println("==============List network success, there had", networks.Count)
	return networks
}

func (n *Neutron) getNetworkPorts(networkId string) []entity.Port {
	urlSuffix := fmt.Sprintf("ports?network_id=%s", networkId)
	resp := n.List(n.headers, urlSuffix)
	var ports entity.Ports
	_ = json.Unmarshal(resp, &ports)

	log.Println("==============List ports success", networkId)
	return ports.Ps
}

func (n *Neutron) DeleteNetwork(ipId string) Output {
	outputObj := Output{ParametersMap: map[string]string{"network_id": ipId}}
	defer func() {
		if err := recover(); err != nil {
			log.Println("catch error：", err)
			outputObj.Success = false
			outputObj.Response = err
		}
	}()

	urlSuffix := fmt.Sprintf("networks/%s", ipId)
	outputObj.Success, outputObj.Response = n.Delete(n.headers, urlSuffix)
	return outputObj
}

func (n *Neutron) DeleteNetworks() {
	networks := n.ListNetworks()
	ch := n.MakeDeleteChannel(consts.NETWORK, len(networks.Nets))

	for _, network := range networks.Nets {
		tempNetwork := network
		go func() {
			ch <- n.DeleteNetwork(tempNetwork.Id)
		}()
	}
	if len(ch) != cap(ch) {
		for len(ch) != cap(ch) {}
	}
	log.Println("Networks were deleted completely")
}

// GetExtNet get the first ext net
func (n *Neutron) GetExtNet() string {
	resp := n.DecorateGetResp(n.Get)(n.headers, "networks?router%3Aexternal=True")
	v, _ := resp["networks"]
	networks := v.([]interface{})
	if len(networks) == 0 {
		log.Println("not ext net to use")
		return ""
	}
	firstNet := networks[0].(map[string]interface{})
	return firstNet["id"].(string)
}

// subnet

func (n *Neutron) CreateSubnet(opts *entity.CreateSubnetOpts) string {
	opts.Name = fmt.Sprintf("%s_%s", opts.Name + strconv.FormatUint(n.snowflake.NextVal(), 10), consts.SUBNET)
	//rand.Seed(time.Now().UnixNano())
	//randomNum := rand.Intn(200)
	//opts.CIDR = fmt.Sprintf("192.%d.%d.0/24", randomNum, randomNum)
	//opts.IPVersion = 4
	reqBody := opts.ToRequestBody()
	resp := n.Post(n.headers, consts.SUBNETS, reqBody)
	var subnet entity.SubnetMap
	_ = json.Unmarshal(resp, &subnet)
	log.Println("==============Create subnet success", subnet.Subnet.Id)
	return subnet.Subnet.Id
}

func (n *Neutron) UpdateSubnet(subnetId, updateBody string) {
	urlSuffix := fmt.Sprintf("subnets/%s", subnetId)
	_ = n.Put(n.headers, urlSuffix, updateBody)

	log.Printf("==============update subnet %s success\n", subnetId)
}

func (n *Neutron) UpdateSubnetHostRoutes (subnetId string) {
   updateBody := fmt.Sprintf("{\"subnet\": {\"host_routes\": [{\"destination\": \"192.168.20.0/24\", \"nexthop\": \"192.168.10.1\"}]}}")
   n.UpdateSubnet(subnetId, updateBody)
}

func (n *Neutron) UpdateSubnetDnsNameservers (subnetId string) {
	updateBody := fmt.Sprintf("{\"subnet\": {\"dns_nameservers \": [\"192.168.20.0/24\"]}}")
	n.UpdateSubnet(subnetId, updateBody)
}

func (n *Neutron) GetSubnet(subnetId string) entity.SubnetMap {
	urlSuffix := fmt.Sprintf("subnets/%s", subnetId)
	resp := n.Get(n.headers, urlSuffix)
	var subnet entity.SubnetMap
	_ = json.Unmarshal(resp, &subnet)
	log.Printf("==============Get subnet success %+v", subnet)
	return subnet
}

func (n *Neutron) ListSubnet() entity.Subnets {
	var urlSuffix string
	if n.isAdmin {
		urlSuffix = consts.SUBNETS
	} else {
		urlSuffix = fmt.Sprintf("subnets?project_id=%s", n.projectId)
	}
	resp := n.List(n.headers, urlSuffix)
	var subnets entity.Subnets
	_ = json.Unmarshal(resp, &subnets)
	log.Println("==============List subnet success")
	return subnets
}

func (n *Neutron) getSubnetCidr(subnetId string) string {
	subnet := n.GetSubnet(subnetId)
	cidr := subnet.Subnet.Cidr
	return cidr
}

func (n *Neutron) DeleteSubnet(ipId string) Output {
	outputObj := Output{ParametersMap: map[string]string{"subnet_id": ipId}}
	defer func() {
		if err := recover(); err != nil {
			log.Println("catch error：", err)
			outputObj.Success = false
			outputObj.Response = err
		}
	}()

	urlSuffix := fmt.Sprintf("%s/%s", consts.SUBNETS, ipId)
	outputObj.Success, outputObj.Response = n.Delete(n.headers, urlSuffix)
	return outputObj
}

func (n *Neutron) DeleteSubnets() {
	subnets := n.ListSubnet()
	ch := n.MakeDeleteChannel(consts.SUBNET, len(subnets.Ss))

	for _, subnet := range subnets.Ss {
		tempSubnet := subnet
		go func() {
			ch <- n.DeleteSubnet(tempSubnet.Id)
		}()
	}
	if len(ch) != cap(ch) {
		for len(ch) != cap(ch) {}
	}
	log.Println("Subnets were deleted completely")
}

//port

func (n *Neutron) CreatePort(opts *entity.CreatePortOpts) string {
	opts.Name = fmt.Sprintf("%s_%s", opts.Name + strconv.FormatUint(n.snowflake.NextVal(), 10), consts.PORT)
	reqBody := opts.ToRequestBody()
	resp := n.Post(n.headers, consts.PORTS, reqBody)
	var port entity.PortMap
	_ = json.Unmarshal(resp, &port)
	log.Println("==============Create port success", port.Port.Id)
	return port.Port.Id
}

func (n *Neutron) updatePort(portId string, reqBody string) {
	urlSuffix := fmt.Sprintf("ports/%s", portId)
	n.Put(n.headers, urlSuffix, reqBody)
	log.Println("==============Update port success", portId)
}

func (n *Neutron) UpdatePortWithSg(portId, sgId string)  {
	reqBody := fmt.Sprintf("{\"port\": {\"security_groups\": [\"%+v\"]}}", sgId)
	n.updatePort(portId, reqBody)
}

func (n *Neutron) UpdatePortWithAllowedAddressPairs(portId, ipaddress, mac string)  {
	reqBody := fmt.Sprintf("{\"port\": {\"allowed_address_pairs\": [{\"ip_address\": \"%+v\", \"mac_address\": \"%+v\"]}}", ipaddress, mac)
	n.updatePort(portId, reqBody)
}

func (n *Neutron) UpdatePortWithMac(portId, mac string)  {
	reqBody := fmt.Sprintf("{\"port\": {\"mac_address\": \"%+v\"}}", mac)
	n.updatePort(portId, reqBody)
}

func (n *Neutron) UpdatePortWithMacNone(portId string)  {
	reqBody := `{"port": {"mac_address": null}}`
	n.updatePort(portId, reqBody)
}

func (n *Neutron) UpdatePortWithQos(portId, qosPolicyId string) {
	reqBody := fmt.Sprintf("{\"port\": {\"qos_policy_id\": \"%+v\"}}", qosPolicyId)
	n.updatePort(portId, reqBody)
}

func (n *Neutron) UpdatePortWithNoQos(portId string) {
	reqBody := fmt.Sprintf("{\"port\": {\"qos_policy_id\": null}}")
	n.updatePort(portId, reqBody)
}

func (n *Neutron) GetPort(portId string) entity.PortMap {
	urlSuffix := fmt.Sprintf("ports/%s", portId)
	resp := n.Get(n.headers, urlSuffix)
	var port entity.PortMap
	_ = json.Unmarshal(resp, &port)
	log.Println("==============Get port success", portId)
	return port
}

func (n *Neutron) GetPortIP(portId string) string {
	urlSuffix := fmt.Sprintf("ports/%s", portId)
	resp := n.Get(n.headers, urlSuffix)
	var port entity.PortMap
	_ = json.Unmarshal(resp, &port)
	log.Println("==============Get port success", portId)
	return port.FixedIps[0].IpAddress
}

func (n *Neutron) ListPort() entity.Ports {
	var urlSuffix string
	if n.isAdmin {
		urlSuffix = consts.PORTS
	} else {
		urlSuffix = fmt.Sprintf("ports?project_id=%s", n.projectId)
	}

	resp := n.Get(n.headers, urlSuffix)
	var ports entity.Ports
	_ = json.Unmarshal(resp, &ports)
	log.Println("==============List port success")
	return ports
}

func (n *Neutron) GetPortByDevice(deviceId, deviceOwner string) *entity.Port {
	urlSuffix := fmt.Sprintf("ports?device_id=%s&device_owner=%s", deviceId, deviceOwner)
	resp := n.List(n.headers, urlSuffix)
	var ports entity.Ports
	_ = json.Unmarshal(resp, &ports)
	if len(ports.Ps) == 0 {
		return nil
	} else {
		return &(ports.Ps[0])
	}
}

func (n *Neutron) DeletePort(ipId string) Output {
	outputObj := Output{ParametersMap: map[string]string{"port_id": ipId}}
	defer func() {
		if err := recover(); err != nil {
			log.Println("catch error：", err)
			outputObj.Success = false
			outputObj.Response = err
		}
	}()

	urlSuffix := fmt.Sprintf("%s/%s", consts.PORTS, ipId)
	outputObj.Success, outputObj.Response = n.Delete(n.headers, urlSuffix)
	return outputObj
}

func (n *Neutron) DeletePorts() {
	ports := n.ListPort()
	ch := n.MakeDeleteChannel(consts.PORT, len(ports.Ps))

	for _, port := range ports.Ps {
		tempPort := port
		go func() {
			ch <- n.DeletePort(tempPort.Id)
		}()
	}
	if len(ch) != cap(ch) {
		for len(ch) != cap(ch) {}
	}
	log.Println("Ports were deleted completely")
}

// router

func (n *Neutron) CreateRouter(opts *entity.CreateRouterOpts) string {
	opts.Name = fmt.Sprintf("%s_%s", opts.Name + strconv.FormatUint(n.snowflake.NextVal(), 10), consts.ROUTER)
	PostBody := opts.ToRequestBody()
	resp := n.Post(n.headers, consts.ROUTERS, PostBody)
	var router entity.RouterMap
	_ = json.Unmarshal(resp, &router)

	//cache.RedisClient.SetMap(n.tag + consts.ROUTERS, router.Router.Id, router)
	log.Println("==============Create router success", router.Router.Id)
	return router.Router.Id
}

func (n *Neutron) UpdateRouter(routerId string, opts *entity.UpdateRouterOpts) string {
	PostBody := opts.ToRequestBody()
	urlSuffix := fmt.Sprintf("routers/%s", routerId)
	resp := n.Put(n.headers, urlSuffix, PostBody)
	var router entity.RouterMap
	_ = json.Unmarshal(resp, &router)

	//cache.RedisClient.SetMap(n.tag + consts.ROUTERS, router.Router.Id, router)
	log.Println("==============Update router success resp", router.Router.Id)
	return router.Router.Id
}

func (n *Neutron) AddRouterInterface(opts *entity.AddRouterInterfaceOpts) string {
	routerId, body := opts.ToRequestBody()
	urlSuffix := fmt.Sprintf("routers/%s/add_router_interface", routerId)
	n.Put(n.headers, urlSuffix, body)

	log.Println("==============Add router interface success")
	return routerId
}

func (n *Neutron) RemoveRouterInterface(routerId, subnetId string) Output {
	outputObj := Output{ParametersMap: map[string]string{"router_id": routerId, "subnetId": subnetId}}
	defer func() {
		if err := recover(); err != nil {
			log.Println("catch error：", err)
			outputObj.Success = false
			outputObj.Response = err
			log.Println("==============Remove router interface failed")
		} else {
			log.Println("==============Remove router interface success")
		}
	}()
	body := fmt.Sprintf("{\"subnet_id\": \"%+v\"}", subnetId)
	urlSuffix := fmt.Sprintf("routers/%s/remove_router_interface", routerId)
	outputObj.Response = string(n.Put(n.headers, urlSuffix, body))
	outputObj.Success = true
	return outputObj
}

func (n *Neutron) DeleteRouterInterfaces() {
	interfacePorts := n.listRouterInterfacePorts()
	ch := n.MakeDeleteChannel(consts.ROUTERINTERFACE, len(interfacePorts.Ps))

	for _, port := range interfacePorts.Ps {
		routerId := port.DeviceId
		fixedIps := port.FixedIps
		for _, fixedIp := range fixedIps {
			tempFixedIp := fixedIp
			go func() {
				ch <- n.RemoveRouterInterface(routerId, tempFixedIp.SubnetId)
			}()
		}
	}
	if len(ch) != cap(ch) {
		for len(ch) != cap(ch) {}
	}
	log.Println("Router interfaces were deleted completely")
}

func (n *Neutron) ListRouters() entity.Routers {
	var urlSuffix string
	if n.isAdmin {
		urlSuffix = consts.ROUTERS
	} else {
		urlSuffix = fmt.Sprintf("routers?project_id=%s", n.projectId)
	}
	resp := n.List(n.headers, urlSuffix)
	var routers entity.Routers
	_ = json.Unmarshal(resp, &routers)
	log.Println("==============List routers success, there had", routers.Count)
	return routers
}

func (n *Neutron) listRouterInterfacePorts() entity.Ports {
	urlSuffix := fmt.Sprintf("ports?device_owner=network:router_interface&project_id=%s", n.projectId)
	resp := n.List(n.headers, urlSuffix)
	var ports entity.Ports
	_ = json.Unmarshal(resp, &ports)
	log.Println("==============List router interface port success, there had", ports.Count)
	return ports
}

func (n *Neutron) updateRouterNoRoutes(id string) Output {
	outputObj := Output{ParametersMap: map[string]string{"router_id": id}}
	defer func() {
		if err := recover(); err != nil {
			log.Println("catch error：", err)
			outputObj.Success = false
			outputObj.Response = err
		}
	}()

	opts := &entity.UpdateRouterOpts{Routes: new([]entity.Route)}
	n.UpdateRouter(id, opts)
	outputObj.Response = ""
	outputObj.Success = true
	return outputObj
}

func (n *Neutron) DeleteRouterRoutes() {
	routers := n.ListRouters()
	length := 0
	for _, router := range routers.Rs {
		if len(router.Routes) != 0 {
			length++
		}

	}
	ch := n.MakeDeleteChannel(consts.ROUTERROUTE, length)

	for _, router := range routers.Rs {
		if len(router.Routes) != 0 {
			go func() {
				ch <- n.updateRouterNoRoutes(router.Id)
			}()
		}
	}
	if len(ch) != cap(ch) {
		for len(ch) != cap(ch) {}
	}
	log.Println("Router routes were deleted completely")
}

func (n *Neutron) DeleteRouter(routerId string) Output {
	outputObj := Output{ParametersMap: map[string]string{"router_id": routerId}}
	defer func() {
		if err := recover(); err != nil {
			log.Println("catch error：", err)
			outputObj.Success = false
			outputObj.Response = err
			log.Println("==============Delete router failed", routerId)
		} else {
			log.Println("==============Delete router success", routerId)
		}
	}()

	urlSuffix := fmt.Sprintf("%s/%s", consts.ROUTERS, routerId)
	outputObj.Success, outputObj.Response = n.Delete(n.headers, urlSuffix)
	return outputObj
}

func (n *Neutron) DeleteRouters() {
	routers := n.ListRouters()
	ch := n.MakeDeleteChannel(consts.ROUTER, len(routers.Rs))
	for _, router := range routers.Rs {
		tempRouter := router
		go func() {
			ch <- n.DeleteRouter(tempRouter.Id)
		}()
	}
	if len(ch) != cap(ch) {
		for len(ch) != cap(ch) {}
	}
	log.Println("Routers were deleted completely")
}

func (n *Neutron) ClearRouterGateway(routerId, extNetId string) Output {
	outputObj := Output{ParametersMap: map[string]string{"router_id": routerId, "ext_net_id": extNetId}}
	defer func() {
		if err := recover(); err != nil {
			log.Println("catch error：", err)
			outputObj.Success = false
			outputObj.Response = err
		}
	}()
	urlSuffix := fmt.Sprintf("routers/%s", routerId)
	reqBody := `{"router": {"external_gateway_info" : {}}}`
    n.Put(n.headers, urlSuffix, reqBody)
    outputObj.Success = true
    outputObj.Response = ""
	log.Println("==============Clear router gateway success, router", routerId)
    return outputObj
}

func (n *Neutron) DeleteRouterGateways() {
	routers := n.ListRouters()
    var length int
	for _, router := range routers.Rs {
		if !reflect.DeepEqual(router.GatewayInfo, nil) {
            length++
		}
	}
		ch := n.MakeDeleteChannel(consts.ROUTERGATEWAY, length)
	for _, router := range routers.Rs {
		if !reflect.DeepEqual(router.GatewayInfo, nil) {
			go func() {
				ch <- n.ClearRouterGateway(router.Id, router.GatewayInfo.NetworkID)
			}()
		}
	}
	if len(ch) != cap(ch) {
		for len(ch) != cap(ch) {}
	}
	log.Println("Router gateways were deleted completely")
}

func (n *Neutron) RemoveRouterInterfaceByPort(routerId, portId string) string {
	body := fmt.Sprintf("{\"port_id\": \"%+v\"}", portId)
	urlSuffix := fmt.Sprintf("routers/%s/remove_router_interface", routerId)
	n.Put(n.headers, urlSuffix, body)
	log.Println("==============Remove router interface success")
	return routerId
}

func (n *Neutron) addExternalGateway(routerId, extNetId string) map[string]interface{} {
	urlSuffix := fmt.Sprintf("routers/%s", routerId)
	reqBody := fmt.Sprintf("{\"router\" : {\"external_gateway_info\": {\"network_id\" : \"%+v\"}}}", extNetId)
	resp := n.DecorateResp(n.Put)(n.headers, urlSuffix, reqBody)
	router := resp["router"].(map[string]interface{})
	log.Println("==============Set router gateway success")
    return router
}

func (n *Neutron) getRouterPorts(routerId string) []interface{} {
	urlSuffix := fmt.Sprintf("ports?device_id=%s", routerId)
	resp := n.DecorateGetResp(n.List)(n.headers, urlSuffix)
	routerPorts := resp["ports"].([]interface{})
	return routerPorts
}

func (n *Neutron) disassociateAnyPort(routerId string) {
    routerPorts := n.getRouterPorts(routerId)
    for _, port := range routerPorts {
    	n.RemoveRouterInterfaceByPort(routerId, port.(map[string]interface{})["id"].(string))
	}
}

func (n *Neutron) GetRouter(routerId string) entity.RouterMap {
	urlSuffix := fmt.Sprintf("routers/%s", routerId)
	resp := n.Get(n.headers, urlSuffix)
	var router entity.RouterMap
	_ = json.Unmarshal(resp, &router)
	log.Println("==============Get router success", routerId)
	return router
}

func (n *Neutron) getRouterExternalIps(routerId string) []string {
	var router entity.RouterMap
	if cache.RedisClient.Exist(routerId) {
		val := cache.RedisClient.GetVal(routerId)
		_ = json.Unmarshal([]byte(val), &router)
	} else {
		router = n.GetRouter(routerId)
	}
	var fixedIps = make([]string, 0)
	if len(router.Router.GatewayInfo.ExternalFixedIPs) != 0 {
		 for _, fixedip := range router.Router.GatewayInfo.ExternalFixedIPs {
		 	fixedIps = append(fixedIps, fixedip.IPAddress)
		 }
	}
	return fixedIps
}

// floating ip

func (n *Neutron) CreateFloatingIP(opts *entity.CreateFipOpts) string {
    urlSuffix := consts.FLOATINGIPS
    createBody := opts.ToRequestBody()
    resp := n.Post(n.headers, urlSuffix, createBody)
    var fip entity.FipMap
    _ = json.Unmarshal(resp, &fip)
	floatingIpId := fip.Floatingip.Id
	//cache.RedisClient.SetMap(n.tag + consts.FLOATINGIPS, floatingIpId, fip)
	log.Printf("==============Create FIP success %+v\n", fip)
	return floatingIpId
}

//func (n *Neutron) CreateFIPExtNet(extNetId string) {
//	createBody := fmt.Sprintf("{\"floatingip\": {\"floating_network_id\": \"%+v\"}}", extNetId)
//	n.createFloatingIP(createBody)
//}

//func (n *Neutron) CreateFIPForInstance(portId string) string {
//	extNetId := configs.CONF.ExternalNetwork
//	createBody := fmt.Sprintf("{\"floatingip\": {\"floating_network_id\": \"%+v\", \"port_id\": \"%+v\"}}", extNetId, portId)
//	return n.createFloatingIP(createBody)
//}

func (n *Neutron) UpdateFloatingIp(fipId, updateBody string) string {
	urlSuffix := fmt.Sprintf("floatingips/%s", fipId)
	resp := n.Put(n.headers, urlSuffix, updateBody)
	var fip entity.FipMap
	_ = json.Unmarshal(resp, &fip)

	log.Println("==============Update FIP success", fipId)
	return fipId
}

func (n *Neutron) UpdateFloatingIpWithQos(fipId, qosId string) string {
	reqBody := fmt.Sprintf("{\"floatingip\": {\"qos_policy_id\": \"%+v\"}}", qosId)
	return n.UpdateFloatingIp(fipId, reqBody)
}

func (n *Neutron) UpdateFloatingIpWithPort(fipId, portId string) string {
	reqBody := fmt.Sprintf("{\"floatingip\": {\"port_id\": \"%+v\"}}", portId)
	return n.UpdateFloatingIp(fipId, reqBody)
}

func (n *Neutron) UpdateFloatingIpWithPortIpAddress(fipId, portId, fixedIp string) string {
	reqBody := fmt.Sprintf("{\"floatingip\": {\"port_id\": \"%+v\", \"fixed_ip_address\": \"%+v\"}}", portId, fixedIp)
	return n.UpdateFloatingIp(fipId, reqBody)
}

func (n *Neutron) FloatingIpDisassociatePort(fipId string) string {
	reqBody := fmt.Sprintf("{\"floatingip\": {\"port_id\": null}}")
	return n.UpdateFloatingIp(fipId, reqBody)
}

func (n *Neutron) GetFIP(fipId string) entity.FipMap {
	urlSuffix := fmt.Sprintf("floatingips/%s", fipId)
	resp := n.Get(n.headers, urlSuffix)
	var fip entity.FipMap
	_ = json.Unmarshal(resp, &fip)
	log.Println("==============Get fip success", fipId)
	return fip
}

func (n *Neutron) ListFIPs() entity.Fips {
	var urlSuffix string
	if n.isAdmin {
		urlSuffix = consts.FLOATINGIPS
	} else {
		urlSuffix = fmt.Sprintf("floatingips?project_id=%s", n.projectId)
	}

	resp := n.List(n.headers, urlSuffix)
	var fs entity.Fips
	_ = json.Unmarshal(resp, &fs)
	log.Println("==============List fip success, there had", fs.Count)
	return fs
}

func (n *Neutron) DeleteFIP(fipId string) Output {
	outputObj := Output{ParametersMap: map[string]string{"floatingip_id": fipId}}
	defer func() {
		if err := recover(); err != nil {
			log.Println("catch error：", err)
			outputObj.Success = false
			outputObj.Response = err
		}
	}()
	urlSuffix := fmt.Sprintf("%s/%s", consts.FLOATINGIPS, fipId)
	outputObj.Success, outputObj.Response = n.Delete(n.headers, urlSuffix)
	return outputObj
}

func (n *Neutron) DeleteFloatingips() {
	fips := n.ListFIPs()
	ch := n.MakeDeleteChannel(consts.FLOATINGIP, len(fips.Fs))

	for _, fip := range fips.Fs {
		tempFip := fip
		go func() {
			ch <- n.DeleteFIP(tempFip.Id)
		}()
	}
	if len(ch) != cap(ch) {
		for len(ch) != cap(ch) {}
	}
	log.Println("Floatingips were deleted completely")
}

// port forwarding

func (n *Neutron) CreatePortForwarding(fipId string, opts *entity.CreatePortForwardingOpts) string {
	urlSuffix := fmt.Sprintf("%s/%s/%s", consts.FLOATINGIPS, fipId, consts.PORTFORWARDINGS)
	createBody := opts.ToRequestBody()
	resp := n.Post(n.headers, urlSuffix, createBody)
	var pf entity.PortForwardingMap
	_ = json.Unmarshal(resp, &pf)

	log.Printf("==============Create port forwarding success %+v\n", pf)
	return pf.PortForwarding.Id
}

func (n *Neutron) GetPortForwarding(fipId string, pfId string) entity.PortForwardingMap {
	urlSuffix := fmt.Sprintf("%s/%s/%s/%s", consts.FLOATINGIPS, fipId, consts.PORTFORWARDINGS, pfId)
	resp := n.Get(n.headers, urlSuffix)
	var pf entity.PortForwardingMap
	_ = json.Unmarshal(resp, &pf)

	log.Printf("==============Get port forwarding success %+v\n", pf)
	return pf
}

func (n *Neutron) ListPortForwarding(fipId string) entity.PortForwardings {
	urlSuffix := fmt.Sprintf("%s/%s/%s", consts.FLOATINGIPS, fipId, consts.PORTFORWARDINGS)
	resp := n.List(n.headers, urlSuffix)
	var pfs entity.PortForwardings
	_ = json.Unmarshal(resp, &pfs)

	log.Printf("==============List port forwarding success %+v\n", pfs)
	return pfs
}

func (n *Neutron) DeletePortForwarding(fipId string, pfId string) Output {
	outputObj := Output{ParametersMap: map[string]string{"floatingip_id": fipId, "port_forwarding_id": pfId}}
	defer func() {
		if err := recover(); err != nil {
			log.Println("catch error：", err)
			outputObj.Success = false
			outputObj.Response = err
		}
	}()
	urlSuffix := fmt.Sprintf("%s/%s/%s/%s", consts.FLOATINGIPS, fipId, consts.PORTFORWARDINGS, pfId)
	outputObj.Success, outputObj.Response = n.Delete(n.headers, urlSuffix)
	return outputObj
}

func (n *Neutron) DeletePortForwardings() {
	fips := n.ListFIPs()
	var pfsMap = make(map[string]entity.PortForwardings)
	var length int
	for _, fip := range fips.Fs {
		tmpPfs := n.ListPortForwarding(fip.Id)
		pfsMap[fip.Id] = tmpPfs
		length += len(tmpPfs.Pfs)
	}

	ch := n.MakeDeleteChannel(consts.PORTFORWARDING, length)
	for fipId, pfs := range pfsMap {
		for _, pf := range pfs.Pfs {
			tmpPf := pf
			go func() {
				ch <- n.DeletePortForwarding(fipId, tmpPf.Id)
			}()
		}
	}

	if len(ch) != cap(ch) {
		for len(ch) != cap(ch) {}
	}
	log.Println("Port forwarding were deleted completely")
}

// qos policy

func (n *Neutron) CreateQos() string {
	urlSuffix := "qos/policies"
	name := "qos_policy_" + strconv.FormatUint(n.snowflake.NextVal(), 10)
	formatter := `{
                      "policy": {
                          "name": "%+v", 
                          "description": "This policy limits the ports to 10Mbit max.",
                          "shared": false
                      }
                   }`
	reqBody := fmt.Sprintf(formatter, name)
	resp := n.Post(n.headers, urlSuffix, reqBody)
	var qos entity.QosPolicyMap
	_ = json.Unmarshal(resp, &qos)
	qosId := qos.Policy.Id

	//cache.RedisClient.SetMap(n.tag + consts.QOS_POLICIES, qosId, qos)
	log.Println("==============Create qos policy success", qosId)
	return qosId
}

func (n *Neutron) GetQos(qosId string) entity.QosPolicyMap {
	urlSuffix := fmt.Sprintf("qos/policies/%s", qosId)
	resp := n.Get(n.headers, urlSuffix)
	var qos entity.QosPolicyMap
	_ = json.Unmarshal(resp, &qos)
	log.Println("==============Get qos policy resp", string(resp))
	return qos
}

func (n *Neutron) CreateBandwidthLimitRuleIngress(qosId string) {
	urlSuffix := fmt.Sprintf("qos/policies/%s/bandwidth_limit_rules", qosId)
	formatter := `{
                      "bandwidth_limit_rule": {
                          "max_kbps": 10240,
                          "direction": "ingress"
                      }
                   }`
	reqBody := formatter
	n.Post(n.headers, urlSuffix, reqBody)
	log.Println("==============Create bandwidth_limit_rule success")
}


func (n *Neutron) CreateBandwidthLimitRuleEgress(qosId string) {
	urlSuffix := fmt.Sprintf("qos/policies/%s/bandwidth_limit_rules", qosId)
	formatter := `{
                      "bandwidth_limit_rule": {
                          "max_kbps": 20480,
                          "direction": "egress"
                      }
                   }`
	reqBody := formatter
	n.Post(n.headers, urlSuffix, reqBody)
	log.Println("==============Create bandwidth_limit_rule success")
}

func (n *Neutron) updateBandwidthLimitRule(qosId string, ruleId string) {
	urlSuffix := fmt.Sprintf("qos/policies/%s/bandwidth_limit_rules/%s", qosId, ruleId)
	reqBody := fmt.Sprintf("{\"bandwidth_limit_rule\": {\"max_kbps\": \"10304\"}}")
	resp := n.Put(n.headers, urlSuffix, reqBody)
	log.Println("==============Update bandwidth_limit_rule success", resp)
}

func (n *Neutron) DeleteBandwidthLimitRules() {
	qoss := n.listQoss()
	var length int
	for _, qos := range qoss.Qps {
		rules := qos.Rules
		for _, rule := range rules {
			if rule.Type == "bandwidth_limit" {
				length++
			}
		}
	}
	ch := n.MakeDeleteChannel(consts.BANDWIDTH_LIMIT_RULE, length)
	for _, qos := range qoss.Qps {
		rules := qos.Rules
		for _, rule := range rules {
			if rule.Type == "bandwidth_limit" {
				go func() {
					ch <- n.DeleteQosRule(rule.Type, qos.Id, rule.Id)
				}()
			}
		}
	}

	if len(ch) != cap(ch) {
		for len(ch) != cap(ch) {}
	}
	log.Println("Bandwidth limit rules were deleted completely")
}

func (n *Neutron) CreateDscpMarkingRule(qosId string) {
	urlSuffix := fmt.Sprintf("qos/policies/%s/dscp_marking_rules", qosId)
	reqBody := fmt.Sprintf("{\"dscp_marking_rule\": {\"dscp_mark\": \"32\"}}")
	resp := n.Post(n.headers, urlSuffix, reqBody)
	log.Println("==============Create dscp_marking_rule success", string(resp))
}

func (n *Neutron) updateDscpMarkingRule(qosId string, ruleId string) {
	urlSuffix := fmt.Sprintf("qos/policies/%s/dscp_marking_rules/%s", qosId, ruleId)
	reqBody := fmt.Sprintf("{\"dscp_marking_rule\": {\"dscp_mark\": \"32\"}}")
	resp := n.Put(n.headers, urlSuffix, reqBody)
	log.Println("==============Update dscp_marking_rule success", resp)

	//n.SyncMap(n.tag + consts.QOS_POLICIES, qosId, func(s string) interface{} {
	//	return n.GetQos(qosId)
	//}, nil)
}

func (n *Neutron) DeleteDscpMarkingRules() {
	qoss := n.listQoss()
	var length int
	for _, qos := range qoss.Qps {
		rules := qos.Rules
		for _, rule := range rules {
			if rule.Type == "dscp_marking" {
				length++
			}
		}
	}
	ch := n.MakeDeleteChannel(consts.DSCP_MARKING_RULE, length)
	for _, qos := range qoss.Qps {
		rules := qos.Rules
		for _, rule := range rules {
			if rule.Type == "dscp_marking" {
				go func() {
					ch <- n.DeleteQosRule(rule.Type, qos.Id, rule.Id)
				}()
			}
		}
	}
	if len(ch) != cap(ch) {
		for len(ch) != cap(ch) {}
	}
	log.Println("Dscp marking rules were deleted completely")
}

func (n *Neutron) CreateMinimumBandwidthRule(qosId string) {
	urlSuffix := fmt.Sprintf("qos/policies/%s/minimum_bandwidth_rules", qosId)
	reqBody := fmt.Sprintf("{\"minimum_bandwidth_rule\": {\"min_kbps\": \"500\", \"direction\": \"egress\"}}")
	resp := n.Post(n.headers, urlSuffix, reqBody)
	log.Println("==============Create minimum_bandwidth_rule success", string(resp))

	//n.SyncMap(n.tag + consts.QOS_POLICIES, qosId, func(s string) interface{} {
	//	return n.getQos(qosId)
	//}, nil)
}

func (n *Neutron) updateMinimumBandwidthRule(qosId string, ruleId string) {
	urlSuffix := fmt.Sprintf("qos/policies/%s/minimum_bandwidth_rules/%s", qosId, ruleId)
	reqBody := fmt.Sprintf("{\"minimum_bandwidth_rule\": {\"min_kbps\": \"12003\"}}")
	resp := n.Put(n.headers, urlSuffix, reqBody)
	log.Println("==============Update minimum_bandwidth_rule success", resp)

	//n.SyncMap(n.tag + consts.QOS_POLICIES, qosId, func(s string) interface{} {
	//	return n.GetQos(qosId)
	//}, nil)
}

func (n *Neutron) DeleteMinimumBandwidthRules() {
	qoses := n.listQoss()
	var length int
	for _, qos := range qoses.Qps {
		rules := qos.Rules
		for _, rule := range rules {
			if rule.Type == "minimum_bandwidth" {
				length++
			}
		}
	}
	ch := n.MakeDeleteChannel(consts.MINIMUM_BANDWIDTH_RULE, length)
	for _, qos := range qoses.Qps {
		rules := qos.Rules
		for _, rule := range rules {
			if rule.Type == "minimum_bandwidth" {
				go func() {
					ch <- n.DeleteQosRule(rule.Type, qos.Id, rule.Id)
				}()
			}
		}
	}
	if len(ch) != cap(ch) {
		for len(ch) != cap(ch) {}
	}
	log.Println("Minimum bandwidth rules were deleted completely")
}

func (n *Neutron) CreateQosAndRule() string {
	qosId := n.CreateQos()

	n.CreateBandwidthLimitRuleIngress(qosId)
	n.CreateDscpMarkingRule(qosId)
	n.CreateMinimumBandwidthRule(qosId)
	return qosId
}

func (n *Neutron) listQoss() entity.QosPolicies {
	var urlSuffix string
	if n.isAdmin {
		urlSuffix =	"qos/policies"
	} else {
		urlSuffix = fmt.Sprintf("ports?project_id=%s", n.projectId)
	}
	resp := n.List(n.headers, urlSuffix)
	var qos entity.QosPolicies
	_ = json.Unmarshal(resp, &qos)
	log.Println("==============List qos policy success, there had", qos.Count)
	return qos
}

func (n *Neutron) DeleteQos(qosId string) Output {
	outputObj := Output{ParametersMap: map[string]string{"qos_policy_id": qosId}}
	defer func() {
		if err := recover(); err != nil {
			log.Println("catch error：", err)
			outputObj.Success = false
			outputObj.Response = err
		}
	}()

	urlSuffix := fmt.Sprintf("qos/policies/%s", qosId)
	outputObj.Success, outputObj.Response = n.Delete(n.headers, urlSuffix)
	return outputObj
}

func (n *Neutron) DeleteQosPolicies() {
	qoses := n.listQoss()
	ch := n.MakeDeleteChannel(consts.QOS_POLICY, len(qoses.Qps))
	for _, qos := range qoses.Qps {
		tempQos := qos
		go func() {
			ch <- n.DeleteQos(tempQos.Id)
		}()
	}
	if len(ch) != cap(ch) {
		for len(ch) != cap(ch) {}
	}
	log.Println("Qos policies were deleted completely")
}

func (n *Neutron) DeleteQosRule(ruleType, qosId, ruleId string) Output {
	var identity string
	switch ruleType {
	case "bandwidth_limit":
		identity = consts.BANDWIDTH_LIMIT_RULES
	case "dscp_marking":
		identity = consts.DSCP_MARKING_RULES
	case "minimum_bandwidth":
		identity = consts.MINIMUM_BANDWIDTH_RULES
	}
	outputObj := Output{ParametersMap: map[string]string{"qos_policy_id": qosId, "rule_id": ruleId}}
	defer func() {
		if err := recover(); err != nil {
			log.Println("catch error：", err)
			outputObj.Success = false
			outputObj.Response = err
		}
	}()

	urlSuffix := fmt.Sprintf("qos/policies/%s/%s/%s", qosId, identity, ruleId)
	outputObj.Success, outputObj.Response = n.Delete(n.headers, urlSuffix)
	return outputObj
}

func (n *Neutron) GetInstancePort(instanceId string) (string, string) {
	urlSuffix := fmt.Sprintf("ports?device_id=%s", instanceId)
	resp := n.DecorateGetResp(n.List)(n.headers, urlSuffix)
	instancePorts := resp["ports"].([]interface{})
	port := instancePorts[0].(map[string]interface{})
	portId := port["id"].(string)
	fixedIps := port["fixed_ips"].([]interface{})
	fixedIp := fixedIps[0].(map[string]interface{})
	ipAddr := fixedIp["ip_address"].(string)
	return portId, ipAddr
}

func (n *Neutron) GetFloatingipPort(fipId string) *entity.Port {
	urlSuffix := fmt.Sprintf("ports?device_id=%s", fipId)
	resp := n.List(n.headers, urlSuffix)
	var ports entity.Ports
	_ = json.Unmarshal(resp, &ports)
	log.Println("==============Get floatingip port success for fip=", fipId, string(resp))
	if len(ports.Ps) != 0 {
		fipPort := ports.Ps[0]
		return &fipPort
	}
	return nil
}

// firewall group v2
func (n *Neutron) createFirewallGroup(ingressPolicy, egressPolicy string) map[string]interface{} {
	urlSuffix := "fwaas/firewall_groups"
	name := "dx_fw_" + strconv.FormatUint(n.snowflake.NextVal(), 10)
	reqBody := fmt.Sprintf("{\"firewall_group\": {\"name\": \"%+v\", \"ingress_firewall_policy_id\": \"%+v\", \"egress_firewall_policy_id\": \"%+v\"}}", name, ingressPolicy, egressPolicy)
	resp := n.DecorateResp(n.Post)(n.headers, urlSuffix, reqBody)
	firewallGroup := resp["firewall_group"].(map[string]interface{})
	firewallGroupId := firewallGroup["id"].(string)

	//cache.RedisClient.AddSliceAndJson(firewallGroupId, n.tag + consts.FIREWALLGROUPS, firewallGroup)
	log.Println("==============Create firewall group success", firewallGroupId)
	return firewallGroup
}

func (n *Neutron) updateFirewallGroup(firewallGroupId, updateBody string) map[string]interface{} {
	urlSuffix := fmt.Sprintf("fwaas/firewall_groups/%s", firewallGroupId)
	resp := n.DecorateResp(n.Put)(n.headers, urlSuffix, updateBody)
	firewallGroup := resp["firewall_group"].(map[string]interface{})

	n.SyncResource(firewallGroupId, nil, firewallGroup)
	log.Println("==============update firewall group success", firewallGroupId)
	return firewallGroup
}

func (n *Neutron) updateFirewallGroupNoPortsNoPolicies(firewallGroupId string) map[string]interface{} {
	urlSuffix := fmt.Sprintf("fwaas/firewall_groups/%s", firewallGroupId)
	updateBody := `{"firewall_group": {"ports": [], "ingress_firewall_policy_id": null, "egress_firewall_policy_id": null}}`
	resp := n.DecorateResp(n.Put)(n.headers, urlSuffix, updateBody)
	firewallGroup := resp["firewall_group"].(map[string]interface{})

	n.SyncResource(firewallGroupId, nil, firewallGroup)
	log.Println("==============update firewall group success", firewallGroupId)
	return firewallGroup
}

func (n *Neutron) constructUpdateWithPorts(ports []string) string {
	en := entity.FirewallGroup{Ports: ports}
	fw := map[string]entity.FirewallGroup{"firewall_group": en}
	reqBody, _ := json.Marshal(fw)
	return string(reqBody)
}

func (n *Neutron) constructUpdateWithPolicy(ingressPolicy, egressPolicy string) string {
	reqBody := fmt.Sprintf("{\"firewall_group\": {\"ingress_firewall_policy_id\": \"%+v\", \"egress_firewall_policy_id\": \"%+v\"}}", ingressPolicy, egressPolicy)
	return reqBody
}

func (n *Neutron) deleteFirewallGroup(firewallGroupId string) {
	urlSuffix := fmt.Sprintf("fwaas/firewall_groups/%s", firewallGroupId)
	ok, _ := n.Delete(n.headers, urlSuffix)
	if ok {
		//cache.RedisClient.DeleteKV(firewallGroupId)
		//cache.RedisClient.RemoveFromSlice(n.tag+consts.FIREWALLGROUPS, firewallGroupId)
	}
	log.Println("==============Delete firewall group success", firewallGroupId)
}

//func (n *Neutron) deleteFirewallGroups()  {
//	//fwIds := cache.RedisClient.GetMembers(n.tag + consts.FIREWALLGROUPS)
//	for _, fwId := range fwIds {
//		val := cache.RedisClient.GetVal(fwId)
//		var fw map[string]interface{}
//		_ = json.Unmarshal([]byte(val), &fw)
//		if ingressPolicy, exist := fw["ingress_firewall_policy_id"]; exist {
//			n.updateFirewallPolicyNoRulesV1(ingressPolicy.(string))
//		}
//		if ingressPolicy, exist := fw["egress_firewall_policy_id"]; exist {
//			n.updateFirewallPolicyNoRulesV1(ingressPolicy.(string))
//		}
//        n.updateFirewallGroupNoRoutersNoPoliciesV1(fwId)
//		n.deleteFirewallGroup(fwId)
//	}
//}

func (n *Neutron) CreateFwFpFr() string {
	fr1 := n.createFirewallRule()
	frId1 := fr1["id"].(string)
	fr2 := n.createFirewallRule()
	frId2 := fr2["id"].(string)

	ingresFp := n.createFirewallPolicy([]string{frId1})
	ingressFpId := ingresFp["id"].(string)
	egressFp := n.createFirewallPolicy([]string{frId2})
	egressFpId := egressFp["id"].(string)
	fw := n.createFirewallGroup(ingressFpId, egressFpId)
	fwId := fw["id"].(string)
	return fwId
}

func (n *Neutron) DeleteFwFpFr() {
    //n.deleteFirewallGroups()
    //n.deleteFirewallPolicies()
    //n.deleteFirewallRules()
}

// firewall policy
func (n *Neutron) createFirewallPolicy(firewallRules []string) map[string]interface{} {
	urlSuffix := "fwaas/firewall_policies"
	name := "dx_fwp_" + strconv.FormatUint(n.snowflake.NextVal(), 10)
    en := entity.FirewallPolicy{Name: name, FirewallRules: firewallRules}
    fp := map[string]entity.FirewallPolicy{"firewall_policy": en}
	reqBody, _ := json.Marshal(fp)

	resp := n.DecorateResp(n.Post)(n.headers, urlSuffix, string(reqBody))
	firewallPolicy := resp["firewall_policy"].(map[string]interface{})
	firewallPolicyId := firewallPolicy["id"].(string)

	//cache.RedisClient.AddSliceAndJson(firewallPolicyId, n.tag + consts.FIREWALLPOLICIES, firewallPolicy)
	log.Println("==============Create firewall policy success", firewallPolicyId)
	return firewallPolicy
}

func (n *Neutron) updateFirewallPolicyNoRules(firewallPolicyId string) map[string]interface{} {
	urlSuffix := fmt.Sprintf("fwaas/firewall_policies/%s", firewallPolicyId)
	reqBody := `{"firewall_policy": {"firewall_rules": []}}`
	resp := n.DecorateResp(n.Put)(n.headers, urlSuffix, reqBody)
	firewallPolicy := resp["firewall_policy"].(map[string]interface{})

	n.SyncResource(firewallPolicyId, nil, firewallPolicy)
	log.Println("==============update firewall policy no rule success", firewallPolicyId)
	return firewallPolicy
}

func (n *Neutron) deleteFirewallPolicy(firewallPolicyId string) {
	n.updateFirewallPolicyNoRules(firewallPolicyId)
	urlSuffix := fmt.Sprintf("fwaas/firewall_policies/%s", firewallPolicyId)
	if ok, _ := n.Delete(n.headers, urlSuffix); ok {
		cache.RedisClient.DeleteKV(firewallPolicyId)
		//cache.RedisClient.RemoveFromSlice(n.tag + consts.FIREWALLPOLICIES, firewallPolicyId)
		log.Println("==============Delete firewall policy success", firewallPolicyId)
	} else {
		log.Println("==============Delete firewall policy failed", firewallPolicyId)
	}
}

//func (n *Neutron) deleteFirewallPolicies()  {
//	fpIds := cache.RedisClient.GetMembers(n.tag + consts.FIREWALLPOLICIES)
//	for _, fpId := range fpIds {
//		n.deleteFirewallPolicy(fpId)
//	}
//}

// firewall rule
func (n *Neutron) createFirewallRule() map[string]interface{} {
	urlSuffix := "fwaas/firewall_rules"
	name := "dx_fwr_" + strconv.FormatUint(n.snowflake.NextVal(), 10)
	action := "allow"
	destinationPort := "80"
	protocol := "tcp"
	reqBody := fmt.Sprintf("{\"firewall_rule\": {\"name\": \"%+v\", \"action\": \"%+v\", \"destination_port\": \"%+v\", \"protocol\": \"%+v\"}}", name, action, destinationPort, protocol)
	resp := n.DecorateResp(n.Post)(n.headers, urlSuffix, reqBody)
	firewallRule := resp["firewall_rule"].(map[string]interface{})
	firewallRuleId := firewallRule["id"].(string)

	//cache.RedisClient.AddSliceAndJson(firewallRuleId, n.tag + consts.FIREWALLRULES, firewallRule)
	log.Println("==============Create firewall rule success", firewallRuleId)
	return firewallRule
}

func (n *Neutron) insertRule(firewallPolicyId, firewallRuleId string) {
	urlSuffix := fmt.Sprintf("fwaas/firewall_policies/%s/insert_rule", firewallPolicyId)
    reqBody := fmt.Sprintf("{\"firewall_rule_id\": \"%+v\", \"insert_after\": \"\", \"insert_before\": \"\"}", firewallRuleId)
    firewallPolicy := n.DecorateResp(n.Put)(n.headers, urlSuffix, reqBody)

    n.SyncResource(firewallPolicyId, nil, firewallPolicy)
	log.Println("==============insert firewall rule success", firewallRuleId)
}

func (n *Neutron) removeRule(firewallPolicyId, firewallRuleId string) {
	urlSuffix := fmt.Sprintf("fwaas/firewall_policies/%s/remove_rule", firewallPolicyId)
	reqBody := fmt.Sprintf("{\"firewall_rule_id\": \"%+v\"}", firewallRuleId)
	firewallPolicy := n.DecorateResp(n.Put)(n.headers, urlSuffix, reqBody)

	n.SyncResource(firewallPolicyId, nil, firewallPolicy)
	log.Println("==============remove firewall rule success", firewallRuleId)
}

func (n *Neutron) deleteFirewallRule(firewallRuleId string) {
	urlSuffix := fmt.Sprintf("fwaas/firewall_rules/%s", firewallRuleId)
	if ok, _ := n.Delete(n.headers, urlSuffix); ok {
		cache.RedisClient.DeleteKV(firewallRuleId)
		//cache.RedisClient.RemoveFromSlice(n.tag + consts.FIREWALLRULES, firewallRuleId)
		log.Println("==============Delete firewall rule success", firewallRuleId)
	} else {
		log.Println("==============Delete firewall rule failed", firewallRuleId)
	}
}

//func (n *Neutron) deleteFirewallRules() {
//	frIds := cache.RedisClient.GetMembers(n.tag + consts.FIREWALLRULES)
//	for _, frId := range frIds {
//		n.deleteFirewallRule(frId)
//	}
//}

// firewall group v1   ***********************************************

func (n *Neutron) CreateFirewallV1(opts *entity.CreateFirewallOpts) string {
	opts.Name = fmt.Sprintf("%s_%s", opts.Name + strconv.FormatUint(n.snowflake.NextVal(), 10), consts.FIREWALL)
	urlSuffix := "fw/firewalls"
	reqBody := opts.ToRequestBody()
	resp := n.Post(n.headers, urlSuffix, reqBody)
	var firewall entity.FirewallV1Map
	_ = json.Unmarshal(resp, &firewall)

	n.ensureFirewallActive(firewall.Id)
	log.Println("==============Create firewall success", firewall.Firewall.Id)
	return firewall.Firewall.Id
}

func (n *Neutron) checkFirewallStatus(firewallId, status string) bool {
	firewall := n.GetFirewall(firewallId)
	if firewall.Status == status {
		return true
	}
	return false
}

func (n *Neutron) ensureFirewallActive(firewallId string) {
	ctx, cancel := context.WithTimeout(context.Background(), consts.Timeout)
	defer cancel()

	for {
		select {
		case <- ctx.Done():
			log.Println("*******************Create firewall timeout")
            return
		default:
			if n.checkFirewallStatus(firewallId, consts.ACTIVE) {
				log.Println("*******************Create firewall success")
				return
			}
			time.Sleep(5 * time.Second)
		}
	}
}

func (n *Neutron) UpdateFirewallV1(firewallId string, opts *entity.UpdateFirewallOpts) entity.FirewallV1Map {
	urlSuffix := fmt.Sprintf("fw/firewalls/%s", firewallId)
	resp := n.Put(n.headers, urlSuffix, opts.ToRequestBody())
	var firewall entity.FirewallV1Map
	_ = json.Unmarshal(resp, &firewall)

	log.Println("==============Update firewall success", firewallId)
	return firewall
}

func (n *Neutron) updateFirewallWithRouterV1(firewallGroupId, routerId string) string {
	urlSuffix := fmt.Sprintf("fw/firewalls/%s", firewallGroupId)
	reqBody := fmt.Sprintf("{\"firewall\": {\"router_id\": \"%+v\"}}", routerId)
	n.Put(n.headers, urlSuffix, reqBody)

	log.Println("==============update firewall success", firewallGroupId)
	return firewallGroupId
}

func (n *Neutron) updateFirewallGroupNoRoutersNoPoliciesV1(firewallGroupId string) map[string]interface{} {
	urlSuffix := fmt.Sprintf("fw/firewalls/%s", firewallGroupId)
	updateBody := `{"firewall": {"router_ids": []}}`
	resp := n.DecorateResp(n.Put)(n.headers, urlSuffix, updateBody)
	firewallGroup := resp["firewall"].(map[string]interface{})

	n.SyncResource(firewallGroupId, nil, firewallGroup)
	log.Println("==============update firewall success", firewallGroupId)
	return firewallGroup
}

func (n *Neutron) ListFirewallV1s() entity.FirewallV1s {
	urlSuffix := fmt.Sprintf("fw/firewalls?project_id=%s", n.projectId)
	resp := n.List(n.headers, urlSuffix)
	var firewalls entity.FirewallV1s
	_ = json.Unmarshal(resp, &firewalls)

	//cache.RedisClient.SetMap(n.tag + consts.FIREWALLGROUPS, firewall.Firewall.Id, firewall)
	log.Println("==============List firewall success, there had", len(firewalls.Fs))
	return firewalls
}

func (n *Neutron) GetFirewall(firewallId string) entity.FirewallV1Map {
	urlSuffix := fmt.Sprintf("fw/firewalls/%s", firewallId)
	resp := n.Get(n.headers, urlSuffix)
	var firewall entity.FirewallV1Map
	_ = json.Unmarshal(resp, &firewall)

	//cache.RedisClient.SetMap(n.tag + consts.FIREWALLGROUPS, firewall.Firewall.Id, firewall)
	log.Printf("==============Get firewall success %+v\n", firewall)
	return firewall
}

func (n *Neutron) deleteFirewallV1(id string) Output {
	outputObj := Output{ParametersMap: map[string]string{"firewall_id": id}}
	defer func() {
		if err := recover(); err != nil {
			log.Println("catch error：", err)
			outputObj.Success = false
			outputObj.Response = err
		}
	}()

	urlSuffix := fmt.Sprintf("fw/firewalls/%s", id)
	outputObj.Success, outputObj.Response = n.Delete(n.headers, urlSuffix)
	return outputObj
}

func (n *Neutron) DeleteFirewalls() {
	fws := n.ListFirewallV1s()
	ch := n.MakeDeleteChannel(consts.FIREWALL, len(fws.Fs))
	for _, fw := range fws.Fs {
		tempFw := fw
		go func() {
			ch <- n.deleteFirewallV1(tempFw.Id)
		}()
	}
	if len(ch) != cap(ch) {
		for len(ch) != cap(ch) {}
	}
	log.Println("Firewalls were deleted completely")
}

// firewall policy

func (n *Neutron) CreateFirewallPolicyV1(opts *entity.CreateFirewallPolicyOpts) string {
	urlSuffix := "fw/firewall_policies"
	opts.Name = fmt.Sprintf("%s_%s", opts.Name + strconv.FormatUint(n.snowflake.NextVal(), 10), consts.FIREWALLPOLICY)
    reqBody := opts.ToRequestBody()
	resp := n.Post(n.headers, urlSuffix, string(reqBody))
	var firewallPolicy entity.FirewallPolicyV1Map
    _ = json.Unmarshal(resp, &firewallPolicy)
	//cache.RedisClient.SetMap(n.tag + consts.FIREWALLPOLICIES, firewallPolicy.FirewallPolicy.Id, firewallPolicy)
	log.Println("==============Create firewall policy success", firewallPolicy.FirewallPolicyV1.Id)
	return firewallPolicy.FirewallPolicyV1.Id
}

func (n *Neutron) updateFirewallPolicyNoRulesV1(firewallPolicyId string) map[string]interface{} {
	urlSuffix := fmt.Sprintf("fw/firewall_policies/%s", firewallPolicyId)
	reqBody := `{"firewall_policy": {"firewall_rules": []}}`
	resp := n.DecorateResp(n.Put)(n.headers, urlSuffix, reqBody)
	firewallPolicy := resp["firewall_policy"].(map[string]interface{})

	n.SyncResource(firewallPolicyId, nil, firewallPolicy)
	log.Println("==============update firewall policy no rule success", firewallPolicyId)
	return firewallPolicy
}

func (n *Neutron) UpdateFirewallPolicyInsertRuleV1(firewallPolicyId, ruleId string) string {
	urlSuffix := fmt.Sprintf("fw/firewall_policies/%s/insert_rule", firewallPolicyId)
	formatter := `{"insert_after": "", "insert_before": "", "firewall_rule_id": "%+v"}`
	reqBody := fmt.Sprintf(formatter, ruleId)
	resp := n.Put(n.headers, urlSuffix, reqBody)
	var policy entity.FirewallPolicyV1Map
    _ = json.Unmarshal(resp, &policy)
	//n.SyncMap(n.tag + consts.FIREWALLPOLICIES, firewallPolicyId, nil, policy)
	log.Println("==============update firewall policy success", firewallPolicyId)
	return firewallPolicyId
}

func (n *Neutron) UpdateFirewallPolicyRemoveRuleV1(firewallPolicyId, ruleId string) string {
	urlSuffix := fmt.Sprintf("fw/firewall_policies/%s/remove_rule", firewallPolicyId)
	formatter := `{"firewall_rule_id": "%+v"}`
	reqBody := fmt.Sprintf(formatter, ruleId)
	resp := n.Put(n.headers, urlSuffix, reqBody)
	var policy entity.FirewallPolicyV1Map
	_ = json.Unmarshal(resp, &policy)
	//n.SyncMap(n.tag + consts.FIREWALLPOLICIES, firewallPolicyId, nil, policy)
	log.Println("==============update firewall policy success", firewallPolicyId)
	return firewallPolicyId
}

func (n *Neutron) updateFirewallPolicyV1(firewallPolicyId string) map[string]interface{} {
	urlSuffix := fmt.Sprintf("fw/firewall_policies/%s", firewallPolicyId)
	reqBody := `{"firewall_policy": {"name": "dx"}}`
	resp := n.DecorateResp(n.Put)(n.headers, urlSuffix, reqBody)
	firewallPolicy := resp["firewall_policy"].(map[string]interface{})

	n.SyncResource(firewallPolicyId, nil, firewallPolicy)
	log.Println("==============update firewall policy success", firewallPolicyId)
	return firewallPolicy
}

func (n *Neutron) GistFirewallPolicy(firewallPolicyId string) entity.FirewallPolicy {
	urlSuffix := fmt.Sprintf("fw/firewall_policies/%s", firewallPolicyId)
	resp := n.List(n.headers, urlSuffix)
	var firewallPolicy entity.FirewallPolicy
	_ = json.Unmarshal(resp, &firewallPolicy)
	log.Printf("==============Get firewall policy success %+v\n", firewallPolicy)
	return firewallPolicy
}

func (n *Neutron) listFirewallPoliciesV1() entity.FirewallPolicies {
	urlSuffix := fmt.Sprintf("fw/firewall_policies?project_id=%s", n.projectId)
	resp := n.List(n.headers, urlSuffix)
	var firewallPolicies entity.FirewallPolicies
	_ = json.Unmarshal(resp, &firewallPolicies)
	//cache.RedisClient.SetMap(n.tag + consts.FIREWALLPOLICIES, firewallPolicy.FirewallPolicy.Id, firewallPolicy)
	log.Println("==============List firewall policy success, there had", len(firewallPolicies.Fps))
	return firewallPolicies
}

func (n *Neutron) deleteFirewallPolicyV1(id string) Output {
	outputObj := Output{ParametersMap: map[string]string{"firewall_policy_id": id}}
	defer func() {
		if err := recover(); err != nil {
			log.Println("catch error：", err)
			outputObj.Success = false
			outputObj.Response = err
		}
	}()

	urlSuffix := fmt.Sprintf("fw/firewall_policies/%s", id)
	outputObj.Success, outputObj.Response = n.Delete(n.headers, urlSuffix)
	return outputObj
}

func (n *Neutron) DeleteFirewallPolicies() {
	fps := n.listFirewallPoliciesV1()
	ch := n.MakeDeleteChannel(consts.FIREWALLPOLICY, len(fps.Fps))
	for _, fp := range fps.Fps {
		tempFp := fp
		go func() {
			ch <- n.deleteFirewallPolicyV1(tempFp.Id)
		}()
	}
	if len(ch) != cap(ch) {
		for len(ch) != cap(ch) {}
	}
	log.Println("Firewall policies were deleted completely")
}

// firewall rule

func (n *Neutron) CreateFirewallRuleV1(opts *entity.CreateFirewallRuleOpts) string {
	opts.Name = fmt.Sprintf("%s_%s", opts.Name + strconv.FormatUint(n.snowflake.NextVal(), 10), consts.FIREWALLRULE)
	urlSuffix := "fw/firewall_rules"
	reqBody := opts.ToRequestBody()
	resp := n.Post(n.headers, urlSuffix, reqBody)
	var firewallRule entity.FirewallRuleV1Map
    _ = json.Unmarshal(resp, &firewallRule)
    //cache.RedisClient.SetMap(n.tag + consts.FIREWALLRULES, firewallRule.FirewallRule.Id, firewallRule)
	log.Println("==============Create firewall rule success", firewallRule.FirewallRuleV1.Id)
	return firewallRule.FirewallRuleV1.Id
}

//func (n *Neutron) CreateFirewallRuleSSHDenyV1() string {
//	name := "dx_fwr_" + strconv.FormatUint(n.snowflake.NextVal(), 10)
//	action := "deny"
//	destinationPort := "22"
//	protocol := "tcp"
//	formatter := `{
//                      "firewall_rule": {
//                          "name": "%+v",
//                          "action": "%+v",
//                          "protocol": "%+v",
//                          "destination_port": "%+v"
//                      }
//                   }`
//	reqBody := fmt.Sprintf(formatter, name, action, protocol, destinationPort)
//	firewallId := n.createFirewallRuleV1(reqBody)
//	return firewallId
//}


func (n *Neutron) insertRuleV1(firewallPolicyId, firewallRuleId string) {
	urlSuffix := fmt.Sprintf("fw/firewall_policies/%s/insert_rule", firewallPolicyId)
	reqBody := fmt.Sprintf("{\"firewall_rule_id\": \"%+v\", \"insert_after\": \"\", \"insert_before\": \"\"}", firewallRuleId)
	firewallPolicy := n.DecorateResp(n.Put)(n.headers, urlSuffix, reqBody)

	n.SyncResource(firewallPolicyId, nil, firewallPolicy)
	log.Println("==============insert firewall rule success", firewallRuleId)
}

func (n *Neutron) removeRuleV1(firewallPolicyId, firewallRuleId string) {
	urlSuffix := fmt.Sprintf("fw/firewall_policies/%s/remove_rule", firewallPolicyId)
	reqBody := fmt.Sprintf("{\"firewall_rule_id\": \"%+v\"}", firewallRuleId)
	firewallPolicy := n.DecorateResp(n.Put)(n.headers, urlSuffix, reqBody)

	n.SyncResource(firewallPolicyId, nil, firewallPolicy)
	log.Println("==============remove firewall rule success", firewallRuleId)
}

func (n *Neutron) listFirewallRulesV1() entity.FirewallRules {
	urlSuffix := fmt.Sprintf("fw/firewall_rules?project_id=%s", n.projectId)
	resp := n.List(n.headers, urlSuffix)
	var firewallRules entity.FirewallRules
	_ = json.Unmarshal(resp, &firewallRules)
	//cache.RedisClient.SetMap(n.tag + consts.FIREWALLPOLICIES, firewallPolicy.FirewallPolicy.Id, firewallPolicy)
	log.Println("==============List firewall rule success, there had", len(firewallRules.Frs))
	return firewallRules
}

func (n *Neutron) DeleteFirewallRuleV1(id string) Output {
	outputObj := Output{ParametersMap: map[string]string{"firewall_rule_id": id}}
	defer func() {
		if err := recover(); err != nil {
			log.Println("catch error：", err)
			outputObj.Success = false
			outputObj.Response = err
		}
	}()

	urlSuffix := fmt.Sprintf("fw/firewall_rules/%s", id)
	outputObj.Success, outputObj.Response = n.Delete(n.headers, urlSuffix)
	return outputObj
}

func (n *Neutron) DeleteFirewallRules() {
	rules := n.listFirewallRulesV1()
	ch := n.MakeDeleteChannel(consts.FIREWALLRULE, len(rules.Frs))
	for _, rule := range rules.Frs {
		tempRule := rule
		go func() {
			ch <- n.DeleteFirewallRuleV1(tempRule.Id)
		}()
	}
	if len(ch) != cap(ch) {
		for len(ch) != cap(ch) {}
	}
	log.Println("Firewall rules were deleted completely")
}

// security group
func (n *Neutron) CreateSecurityGroup(opts *entity.CreateSecurityGroupOpts) string {
	urlSuffix := "security-groups"
	reqBody := opts.ToRequestBody()
	resp := n.Post(n.headers, urlSuffix, reqBody)
	var sg entity.Sg
	_ = json.Unmarshal(resp, &sg)

	//cache.RedisClient.SetMap(n.tag + consts.SECURITYGROUPS, sg.SecurityGroup.Id, sg)
	log.Println("==============Create security group success", sg.SecurityGroup.Id)
	return sg.SecurityGroup.Id
}

func (n *Neutron) getSecurityGroup(sgId string) interface{} {
	urlSuffix := fmt.Sprintf("security-groups/%s", sgId)
    resp := n.Get(n.headers, urlSuffix)
    var sg entity.Sg
    _ = json.Unmarshal(resp, &sg)
    log.Println(fmt.Sprintf("get sg==%+v", sg))
    return sg
}

func (n *Neutron) listSecurityGroupByName(sgName string) entity.Sgs {
	urlSuffix := fmt.Sprintf("security-groups?name=%s", sgName)
	resp := n.List(n.headers, urlSuffix)
	var sgs entity.Sgs
	_ = json.Unmarshal(resp, &sgs)
	log.Println("==============List sg success, there had", sgs.Count)
	return sgs
}

func (n *Neutron) listSecurityGroups() entity.Sgs {
	urlSuffix := fmt.Sprintf("security-groups?project_id=%s", n.projectId)
	resp := n.List(n.headers, urlSuffix)
	var sgs entity.Sgs
	_ = json.Unmarshal(resp, &sgs)
	log.Println("==============List sg success, there had", len(sgs.Sgs))
	return sgs
}

func (n *Neutron) CreateSecurityGroupAndRules(opts *entity.CreateSecurityGroupOpts) string {
    sgId := n.CreateSecurityGroup(opts)
    n.createSecurityGroupRuleICMP(sgId)
    n.createSecurityGroupRuleSSH(sgId)

    //n.SyncMap(n.tag + consts.SECURITYGROUPS, sgId, func(resourceId string) interface{} {
	//	return n.getSecurityGroup(sgId)
	//}, nil)
    return sgId
}

func (n *Neutron) deleteSecurityGroup(id string) Output {
	outputObj := Output{ParametersMap: map[string]string{"security_group_id": id}}
	defer func() {
		if err := recover(); err != nil {
			log.Println("catch error：", err)
			outputObj.Success = false
			outputObj.Response = err
		}
	}()

	urlSuffix := fmt.Sprintf("security-groups/%s", id)
	outputObj.Success, outputObj.Response = n.Delete(n.headers, urlSuffix)
	return outputObj
}

func (n *Neutron) DeleteSecurityGroups() {
	sgs := n.listSecurityGroups()
	ch := n.MakeDeleteChannel(consts.SECURITYGROUP, len(sgs.Sgs))
	for _, sg := range sgs.Sgs {
		tempSg := sg
		go func() {
			ch <- n.deleteSecurityGroup(tempSg.Id)
		}()
	}
	if len(ch) != cap(ch) {
		for len(ch) != cap(ch) {}
	}
	log.Println("Security groups were deleted completely")
}

func (n *Neutron) DeleteSecurityGroupAndRules()  {
	n.DeleteSecurityGroupRules()
	n.DeleteSecurityGroups()
}

//func (n *Neutron) MakeSureSgExist(name string) {
//	if cache.RedisClient.Exist(n.tag + consts.SECURITYGROUPS) {
//		sgMaps := cache.RedisClient.GetMaps(n.tag + consts.SECURITYGROUPS)
//		for _, sgId := range sgMaps {
//			var sg entity.Sg
//			val := cache.RedisClient.GetMap(n.tag + consts.SECURITYGROUPS, sgId)
//			_ = json.Unmarshal([]byte(val), &sg)
//			if sg.Name == name {
//				return
//			}
//		}
//	}
//	n.CreateSecurityGroupAndRules()
//}

func (n *Neutron) EnsureSgExist(sgName string) {
	sgs := n.listSecurityGroupByName(sgName)
	if sgs.Count == 0 {
		n.CreateSecurityGroupAndRules(&entity.CreateSecurityGroupOpts{Name: sgName})
	}
}

// security group rule
func (n *Neutron) CreateSecurityGroupRule(reqBody string) {
	urlSuffix := "security-group-rules"
	resp := n.Post(n.headers, urlSuffix, reqBody)
	var sgRule entity.SgRule
	_ = json.Unmarshal(resp, &sgRule)

	//cache.RedisClient.SetMap(n.tag + consts.SECURITYGROUPRULES, sgRule.SecurityGroupRule.Id, sgRule)
	log.Println("==============Create security group rule success", sgRule.SecurityGroupRule.Id)
}

func (n *Neutron) createSecurityGroupRuleICMP(sgId string) {
	ingress, egress := n.constructICMPRule(sgId)
	n.CreateSecurityGroupRule(ingress)
	n.CreateSecurityGroupRule(egress)
}

func (n *Neutron) constructICMPRule(sgId string) (string, string) {
	ingress := entity.CreateSecurityRuleOpts{Direction: "ingress", EtherType: "IPv4", Protocol: "icmp", SecGroupID: sgId}
	egress := entity.CreateSecurityRuleOpts{Direction: "egress", EtherType: "IPv4", Protocol: "icmp", SecGroupID: sgId}
	return ingress.ToRequestBody(), egress.ToRequestBody()
}

func (n *Neutron) createSecurityGroupRuleSSH(sgId string) {
	ingress, egress := n.constructSSHRule(sgId)
	n.CreateSecurityGroupRule(ingress)
	n.CreateSecurityGroupRule(egress)
}

func (n *Neutron) constructSSHRule(sgId string) (string, string) {
	ingress := entity.CreateSecurityRuleOpts{Direction: "ingress", EtherType: "IPv4", Protocol: "tcp", SecGroupID: sgId, PortRangeMin: 22, PortRangeMax: 22}
	egress := entity.CreateSecurityRuleOpts{Direction: "egress", EtherType: "IPv4", Protocol: "tcp", SecGroupID: sgId, PortRangeMin: 22, PortRangeMax: 22}
	return ingress.ToRequestBody(), egress.ToRequestBody()
}

func (n *Neutron) listSecurityGroupRules() entity.SgRules {
	urlSuffix := fmt.Sprintf("security-group-rules?project_id=%s", n.projectId)
	resp := n.List(n.headers, urlSuffix)
	var sgRules entity.SgRules
	_ = json.Unmarshal(resp, &sgRules)

	//cache.RedisClient.SetMap(n.tag + consts.SECURITYGROUPRULES, sgRule.SecurityGroupRule.Id, sgRule)
	log.Println("==============List sg rule success, there had", len(sgRules.Srs))
	return sgRules
}

func (n *Neutron) deleteSecurityGroupRule(id string) Output {
	outputObj := Output{ParametersMap: map[string]string{"security_group_id": id}}
	defer func() {
		if err := recover(); err != nil {
			log.Println("catch error：", err)
			outputObj.Success = false
			outputObj.Response = err
		}
	}()

	urlSuffix := fmt.Sprintf("security-group-rules/%s", id)
	outputObj.Success, outputObj.Response = n.Delete(n.headers, urlSuffix)
	return outputObj
}

func (n *Neutron) DeleteSecurityGroupRules() {
	sgRules := n.listSecurityGroupRules()
	ch := n.MakeDeleteChannel(consts.SECURITYGROUPRULE, len(sgRules.Srs))
	for _, sgRule := range sgRules.Srs {
		tempSgRule := sgRule
		go func() {
			ch <- n.deleteSecurityGroupRule(tempSgRule.Id)
		}()
	}
	if len(ch) != cap(ch) {
		for len(ch) != cap(ch) {}
	}
	log.Println("Security group rules were deleted completely")
}

// rbac policy

func (n *Neutron) CreateRbacPolicy(objectType, objectId, targetTenant string) string {
	urlSuffix := "rbac-policies"
	reqBody := fmt.Sprintf("{\"rbac_policy\": {\"action\": \"access_as_shared\", \"object_type\": \"%+v\", \"target_tenant\": \"%+v\", \"object_id\": \"%+v\"}}", objectType, targetTenant, objectId)
	resp := n.Post(n.headers, urlSuffix, reqBody)
	var rp entity.RbacPolicyMap
	_ = json.Unmarshal(resp, &rp)

	//cache.RedisClient.SetMap(n.tag + consts.RBACPOLICIES, rp.RbacPolicy.Id, rp)
	log.Println("==============Create rbac policy success", rp.RbacPolicy.Id)
	return rp.RbacPolicy.Id
}

func (n *Neutron) CreateNetworkRbacPolicy(objectId, targetTenant string) string {
	rbacPolicyId := n.CreateRbacPolicy(consts.NETWORK, objectId, targetTenant)
	return rbacPolicyId
}

func (n *Neutron) createQosRbacPolicy(objectId, targetTenant string) string {
	rbacPolicyId := n.CreateRbacPolicy(consts.QOS_POLICY, objectId, targetTenant)
	return rbacPolicyId
}

func (n *Neutron) createSGRbacPolicy(objectId, targetTenant string) string {
	rbacPolicyId := n.CreateRbacPolicy(consts.SECURITYGROUP, objectId, targetTenant)
	return rbacPolicyId
}

func (n *Neutron) deleteRbacPolicy(rbacPolicyId string) {
	urlSuffix := fmt.Sprintf("rbac-policies/%s", rbacPolicyId)
	if ok, _ := n.Delete(n.headers, urlSuffix); ok {
		//cache.RedisClient.DeleteMap(n.tag + consts.RBACPOLICIES, rbacPolicyId)
		log.Println("==============Delete rbac policy success", rbacPolicyId)
		return
	}
	log.Println("==============Delete rbac policy failed", rbacPolicyId)
}

func (n *Neutron) getRbacPolicy(rbacPolicyId string) interface{} {
	urlSuffix := fmt.Sprintf("rbac-policies/%s", rbacPolicyId)
	resp := n.Get(n.headers, urlSuffix)
	var rp entity.RbacPolicyMap
	_ = json.Unmarshal(resp, &rp)
	log.Println(fmt.Sprintf("rbac policy==%+v", rp))
	return rp
}

func (n *Neutron) deleteRbacPolicies() {
	//sgIds := cache.RedisClient.GetMaps(n.tag + consts.RBACPOLICIES)
	//for _, sgId := range sgIds {
	//	n.deleteRbacPolicy(sgId)
	//}
}

// vpn

func (n *Neutron) CreateVpnService(routerId string) string {
	urlSuffix := "vpn/vpnservices"
	name := "vpn_service_" + strconv.FormatUint(n.snowflake.NextVal(), 10)
	reqBody := fmt.Sprintf("{\"vpnservice\": {\"name\": \"%+v\", \"router_id\": \"%+v\"}}", name, routerId)
	resp := n.Post(n.headers, urlSuffix, reqBody)
	var vs entity.VpnServiceMap
	_ = json.Unmarshal(resp, &vs)

	//cache.RedisClient.SetMap(n.tag + consts.VPNSERVICES, vs.VpnService.Id, vs)
	log.Println("==============Create vpn service success", vs.VpnService.Id)
	return vs.VpnService.Id
}

func (n *Neutron) deleteVpnService(vpnServiceId string) {
	defer n.wg.Done()
	urlSuffix := fmt.Sprintf("vpn/vpnservices/%s", vpnServiceId)
	if ok, _ := n.Delete(n.headers, urlSuffix); ok {
		//cache.RedisClient.DeleteMap(n.tag + consts.VPNSERVICES, vpnServiceId)
		log.Println("==============Delete vpn service success", vpnServiceId)
		return
	}
	log.Println("==============Delete vpn service failed", vpnServiceId)
}

func (n *Neutron) getVpnService(vpnServiceId string) entity.VpnServiceMap {
	urlSuffix := fmt.Sprintf("vpn/vpnservices/%s", vpnServiceId)
	resp := n.Get(n.headers, urlSuffix)
	var vs entity.VpnServiceMap
	_ = json.Unmarshal(resp, &vs)
	return vs
}

func (n *Neutron) listVpnServices() entity.VpnServices {
	urlSuffix := fmt.Sprintf("vpn/vpnservices?project_id=%s", n.projectId)
	resp := n.List(n.headers, urlSuffix)
	var ips entity.VpnServices
	_ = json.Unmarshal(resp, &ips)
	log.Println("==============List vpn service success, there had", ips.Count)
	return ips
}

func (n *Neutron) DeleteVpnServices() {
	//vsIds := cache.RedisClient.GetMaps(n.tag + consts.VPNSERVICES)
	vss := n.listVpnServices()
	for _, vs := range vss.Vss {
		n.wg.Add(1)
		go n.deleteVpnService(vs.Id)
	}
	n.wg.Wait()
}

// endpoint group
func (n *Neutron) createEndpointGroup(reqBody string) string {
	urlSuffix := "vpn/endpoint-groups"
	resp := n.Post(n.headers, urlSuffix, reqBody)
	var eg entity.EndpointGroupMap
	_ = json.Unmarshal(resp, &eg)

	//cache.RedisClient.SetMap(n.tag + consts.ENDPOINTGROUPS, eg.EndpointGroup.Id, eg)
	log.Println("==============Create endpoint group success", eg.EndpointGroup.Id)
	return eg.EndpointGroup.Id
}

func (n *Neutron) CreateLocalEndpointGroup(subnetId string) string {
	name := "endpoint_group_" + strconv.FormatUint(n.snowflake.NextVal(), 10)
	formatter := `{
                      "endpoint_group": {
                          "name": "%+v",
                          "endpoints": ["%+v"],
                          "type": "subnet"
                      }
                   }`
	reqBody := fmt.Sprintf(formatter, name, subnetId)
	return n.createEndpointGroup(reqBody)
}

func (n *Neutron) createLocalEndpointGroupTwoSubnet(subnetId1, subnetId2 string) string {
	name := "endpoint_group_" + strconv.FormatUint(n.snowflake.NextVal(), 10)
	reqBody := fmt.Sprintf("{\"endpoint_group\": {\"name\": \"%+v\", \"endpoints\": [\"%+v\", \"%+v\"], \"type\": \"subnet\"}}", name, subnetId1, subnetId2)
	return n.createEndpointGroup(reqBody)
}

func (n *Neutron) createPeerEndpointGroupWithSubnet(subnetId string) string {
	cidr := n.getSubnetCidr(subnetId)
	name := "endpoint_group_" + strconv.FormatUint(n.snowflake.NextVal(), 10)
	formatter := `{
                      "endpoint_group": {
                          "name": "%+v",
                          "endpoints": ["%+v"],
                          "type": "cidr"
                      }
                   }`
	reqBody := fmt.Sprintf(formatter, name, cidr)
	return n.createEndpointGroup(reqBody)
}

func (n *Neutron) CreatePeerEndpointGroup(peerCidr string) string {
	name := "endpoint_group_" + strconv.FormatUint(n.snowflake.NextVal(), 10)
	formatter := `{
                      "endpoint_group": {
                          "name": "%+v",
                          "endpoints": ["%+v"],
                          "type": "cidr"
                      }
                   }`
	reqBody := fmt.Sprintf(formatter, name, peerCidr)
	return n.createEndpointGroup(reqBody)
}

func (n *Neutron) DeleteEndpointGroup(egId string) {
	defer n.wg.Done()
	urlSuffix := fmt.Sprintf("vpn/endpoint-groups/%s", egId)
	if ok, _ := n.Delete(n.headers, urlSuffix); ok {
		//cache.RedisClient.DeleteMap(n.tag + consts.ENDPOINTGROUPS, egId)
		log.Println("==============Delete endpoint group success", egId)
		return
	}
	log.Println("==============Delete endpoint group failed", egId)
}

func (n *Neutron) getEndpointGroup(egId string) entity.EndpointGroupMap {
	urlSuffix := fmt.Sprintf("vpn/endpoint-groups/%s", egId)
	resp := n.Get(n.headers, urlSuffix)
	var eg entity.EndpointGroupMap
	_ = json.Unmarshal(resp, &eg)
	return eg
}

func (n *Neutron) listEndpointGroups() entity.EndpointGroups {
	urlSuffix := fmt.Sprintf("vpn/endpoint-groups?project_id=%s", n.projectId)
	resp := n.List(n.headers, urlSuffix)
	var ips entity.EndpointGroups
	_ = json.Unmarshal(resp, &ips)
	log.Println("==============List endpoint group success, there had", ips.Count)
	return ips
}

func (n *Neutron) DeleteEndpointGroups() {
	//egIds := cache.RedisClient.GetMaps(n.tag + consts.ENDPOINTGROUPS)
	egs := n.listEndpointGroups()
	for _, eg := range egs.Egs {
		n.wg.Add(1)
		go n.DeleteEndpointGroup(eg.Id)
	}
	n.wg.Wait()
}

// ike policy

func (n *Neutron) CreateIkePolicy() string {
	urlSuffix := "vpn/ikepolicies"
	name := "ike_policy_" + strconv.FormatUint(n.snowflake.NextVal(), 10)
	formatter := `{
                      "ikepolicy": {
                          "phase1_negotiation_mode": "main",
                          "auth_algorithm": "sha1",
                          "encryption_algorithm": "aes-128",
                          "pfs": "group14", 
                          "lifetime": {
                              "units": "seconds", 
                              "value": 7200
                          }, 
                          "ike_version": "v1",
                          "name": "%+v"
                      }
                   }`
	reqBody := fmt.Sprintf(formatter, name)
	resp := n.Post(n.headers, urlSuffix, reqBody)
	var ip entity.IkePolicyMap
	_ = json.Unmarshal(resp, &ip)

	//cache.RedisClient.SetMap(n.tag + consts.IKEPOLICIES, ip.Ikepolicy.Id, ip)
	log.Println("==============Create ike policy success", ip.Ikepolicy.Id)
	return ip.Ikepolicy.Id
}

func (n *Neutron) DeleteIkePolicy(ipId string) {
	defer n.wg.Done()
	urlSuffix := fmt.Sprintf("vpn/ikepolicies/%s", ipId)
	if ok, _ := n.Delete(n.headers, urlSuffix); ok {
		//cache.RedisClient.DeleteMap(n.tag + consts.IKEPOLICIES, ipId)
		log.Println("==============Delete ike policy success", ipId)
		return
	}
	log.Println("==============Delete ike policy failed", ipId)
}

func (n *Neutron) getIkePolicy(ipId string) entity.IkePolicyMap {
	urlSuffix := fmt.Sprintf("vpn/ikepolicies/%s", ipId)
	resp := n.Get(n.headers, urlSuffix)
	var ip entity.IkePolicyMap
	_ = json.Unmarshal(resp, &ip)
	return ip
}

func (n *Neutron) listIkePolicies() entity.IpsecPolicies {
	urlSuffix := fmt.Sprintf("vpn/ikepolicies?project_id=%s", n.projectId)
	resp := n.List(n.headers, urlSuffix)
	var ips entity.IpsecPolicies
	_ = json.Unmarshal(resp, &ips)
	log.Println("==============List ike policy success, there had", ips.Count)
	return ips
}

func (n *Neutron) DeleteIkePolicies() {
	//ipIds := cache.RedisClient.GetMaps(n.tag + consts.IKEPOLICIES)
	ipIds := n.listIkePolicies()
	for _, ip := range ipIds.Ips {
		n.wg.Add(1)
		go n.DeleteIkePolicy(ip.Id)
	}
	n.wg.Wait()
}

// ipsec policy

func (n *Neutron) CreateIpsecPolicy() string {
	urlSuffix := "vpn/ipsecpolicies"
	name := "ipsec_policy_" + strconv.FormatUint(n.snowflake.NextVal(), 10)
	formatter := `{
                      "ipsecpolicy": {
                          "transform_protocol": "esp",
                          "auth_algorithm": "sha1",
                          "encapsulation_mode": "tunnel",
                          "encryption_algorithm": "aes-128",
                          "pfs": "group5", 
                          "lifetime": {
                              "units": "seconds", 
                              "value": 7200
                          }, 
                          "name": "%+v"
                      }
                   }`
	reqBody := fmt.Sprintf(formatter, name)
	resp := n.Post(n.headers, urlSuffix, reqBody)
	var ip entity.IpsecPolicyMap
	_ = json.Unmarshal(resp, &ip)

	//cache.RedisClient.SetMap(n.tag + consts.IPSECPOLICIES, ip.Ipsecpolicy.Id, ip)
	log.Println("==============Create ipsec policy success", ip.Ipsecpolicy.Id)
	return ip.Ipsecpolicy.Id
}

func (n *Neutron) deleteIpsecPolicy(ipId string) {
	defer n.wg.Done()
	urlSuffix := fmt.Sprintf("vpn/ipsecpolicies/%s", ipId)
	if ok, _ := n.Delete(n.headers, urlSuffix) ; ok{
		//cache.RedisClient.DeleteMap(n.tag + consts.IPSECPOLICIES, ipId)
		log.Println("==============Delete ipsec policy success", ipId)
		return
	}
	log.Println("==============Delete ipsec policy failed", ipId)
}

func (n *Neutron) getIpsecPolicy(ipId string) entity.IpsecPolicyMap {
	urlSuffix := fmt.Sprintf("vpn/ipsecpolicies/%s", ipId)
	resp := n.Get(n.headers, urlSuffix)
	var ip entity.IpsecPolicyMap
	_ = json.Unmarshal(resp, &ip)
	return ip
}

func (n *Neutron) listIpsecPolicies() entity.IpsecPolicies {
	urlSuffix := fmt.Sprintf("vpn/ipsecpolicies?project_id=%s", n.projectId)
	resp := n.List(n.headers, urlSuffix)
	var ips entity.IpsecPolicies
	_ = json.Unmarshal(resp, &ips)
	log.Println("==============List ipsec policy success, there had", ips.Count)
	return ips
}

func (n *Neutron) DeleteIpsecPolicies() {
	//ipIds := cache.RedisClient.GetMaps(n.tag + consts.IPSECPOLICIES)
    ips := n.listIpsecPolicies()
	for _, ip := range ips.Ips {
		n.wg.Add(1)
		go n.deleteIpsecPolicy(ip.Id)
	}
	n.wg.Wait()
}

//ipsec site connection

// CreateIpsecConnection create
func (n *Neutron) CreateIpsecConnection(vpnServiceId, ikePolicyId, ipsecPolicyId, peerEGId, localEGId, peerAddress string) string {
	urlSuffix := "vpn/ipsec-site-connections"
	name := "ipsec_connection_" + strconv.FormatUint(n.snowflake.NextVal(), 10)
	formatter := `{
                      "ipsec_site_connection": {
                          "initiator": "bi-directional",
                          "ipsecpolicy_id": "%+v",
                          "admin_state_up": true,
                          "mtu": "1500",
                          "psk": "secret", 
                          "peer_ep_group_id": "%+v",
                          "ikepolicy_id": "%+v", 
                          "vpnservice_id": "%+v", 
                          "local_ep_group_id": "%+v", 
                          "peer_address": "%+v",
                          "peer_id": "%+v", 
                          "name": "%+v"
                      }
                   }`
	reqBody := fmt.Sprintf(formatter, ipsecPolicyId, peerEGId, ikePolicyId, vpnServiceId, localEGId, peerAddress, peerAddress, name)
	resp := n.Post(n.headers, urlSuffix, reqBody)
	var ip entity.IpsecConnectionMap
	_ = json.Unmarshal(resp, &ip)

	//cache.RedisClient.SetMap(n.tag + consts.IPSECCONNECTIONS, ip.IpsecSiteConnection.Id, ip)
	log.Println("==============Create ipsec connection success", ip.IpsecSiteConnection.Id)
	return ip.IpsecSiteConnection.Id
}

func (n *Neutron) deleteIpsecConnection(ipId string) {
	defer n.wg.Done()
	urlSuffix := fmt.Sprintf("vpn/ipsec-site-connections/%s", ipId)
	if ok, _ := n.Delete(n.headers, urlSuffix); ok {
		//cache.RedisClient.DeleteMap(n.tag + consts.IPSECCONNECTIONS, ipId)
		log.Println("==============Delete ipsec connection success", ipId)
		return
	}
	log.Println("==============Delete ipsec connection failed", ipId)
}

func (n *Neutron) getIpsecConnection(ipId string) entity.IpsecConnectionMap {
	urlSuffix := fmt.Sprintf("vpn/ipsec-site-connections/%s", ipId)
	resp := n.Get(n.headers, urlSuffix)
	var ip entity.IpsecConnectionMap
	_ = json.Unmarshal(resp, &ip)
	return ip
}

func (n *Neutron) listIpsecConnection() entity.IpsecConnections {
	urlSuffix := fmt.Sprintf("vpn/ipsec-site-connections?project_id=%s", n.projectId)
	resp := n.List(n.headers, urlSuffix)
	var ics entity.IpsecConnections
	_ = json.Unmarshal(resp, &ics)
	log.Println("==============List ipsec connection success, there had", ics.Count)
	return ics
}

func (n *Neutron) DeleteIpsecConnections() {
	//ipIds := cache.RedisClient.GetMaps(n.tag + consts.IPSECCONNECTIONS)
	ics := n.listIpsecConnection()
	for _, ic := range ics.ICs {
		n.wg.Add(1)
		go n.deleteIpsecConnection(ic.Id)
	}
	n.wg.Wait()
}

// vpc connection

func (n *Neutron) CreateVpcConnection(opts *entity.CreateVpcConnectionOpts) string {
	urlSuffix := "vpc-connections"
	opts.Name = fmt.Sprintf("%s_%s", opts.Name + strconv.FormatUint(n.snowflake.NextVal(), 10), consts.VpcConnection)
	reqBody := opts.ToRequestBody()
	resp := n.Post(n.headers, urlSuffix, reqBody)
	var vc entity.VpcConnectionMap
	_ = json.Unmarshal(resp, &vc)

	//cache.RedisClient.SetMap(n.tag + consts.VpcConnections, vc.VpcConnection.Id, vc)
	log.Println("==============Create vpc connection success", vc.VpcConnection.Id)
	return vc.VpcConnection.Id
}

func (n *Neutron) UpdateVpcConnection(vpcConnId string, opts *entity.UpdateVpcConnectionOpts) string {
	urlSuffix := fmt.Sprintf("vpc-connections/%s", vpcConnId)
	reqBody := opts.ToRequestBody()
	resp := n.Put(n.headers, urlSuffix, reqBody)
	var vc entity.VpcConnectionMap
	_ = json.Unmarshal(resp, &vc)

	log.Println("==============Update vpc connection success", vc.VpcConnection.Id)
	return vc.VpcConnection.Id
}

func (n *Neutron) GetVpcConnection(ipId string) entity.VpcConnectionMap {
	urlSuffix := fmt.Sprintf("vpc-connections/%s", ipId)
	resp := n.Get(n.headers, urlSuffix)
	var ip entity.VpcConnectionMap
	_ = json.Unmarshal(resp, &ip)
	log.Println("==============Get vpc connection success", ipId)
	return ip
}

func (n *Neutron) ListVpcConnections() entity.VpcConnections {
	urlSuffix := fmt.Sprintf("vpc-connections?tenant_id=%s", n.projectId)
	resp := n.List(n.headers, urlSuffix)
	var ics entity.VpcConnections
	_ = json.Unmarshal(resp, &ics)
	log.Println("==============List vpc connection success, there had", len(ics.Vcs))
	return ics
}

func (n *Neutron) DeleteVpcConnection(id string) Output {
	outputObj := Output{ParametersMap: map[string]string{"security_group_id": id}}
	defer func() {
		if err := recover(); err != nil {
			log.Println("catch error：", err)
			outputObj.Success = false
			outputObj.Response = err
		}
	}()

	urlSuffix := fmt.Sprintf("vpc-connections/%s", id)
	outputObj.Success, outputObj.Response = n.Delete(n.headers, urlSuffix)
	return outputObj
}

func (n *Neutron) DeleteVpcConnections() {
	vcs := n.ListVpcConnections()
	ch := n.MakeDeleteChannel(consts.VpcConnection, len(vcs.Vcs))
	for _, vc := range vcs.Vcs {
		tempVc := vc
		go func() {
			ch <- n.DeleteVpcConnection(tempVc.Id)
		}()
	}
	if len(ch) != cap(ch) {
		for len(ch) != cap(ch) {}
	}
	log.Println("Vpc connections were deleted completely")
}

// ac plugin api

// compare_results

func (n *Neutron) GetCompareResults() {
	urlSuffix := "compare_results"
	resp := n.List(n.headers, urlSuffix)
	log.Println(string(resp))
}

func (n *Neutron) GetSyncResults() {
	urlSuffix := "sync_results"
	resp := n.List(n.headers, urlSuffix)
	log.Println(string(resp))
	var syncResults entity.SyncResultsMap
	_ = json.Unmarshal(resp, &syncResults)
	log.Println("==============List sync results success")
}

func (n *Neutron) GetSyncSummaryResults() {
	urlSuffix := "sync_results?sync_summary=true"
	resp := n.List(n.headers, urlSuffix)
	log.Println(string(resp))
	var syncResults entity.SyncSummaryResult
	_ = json.Unmarshal(resp, &syncResults)
	log.Println("==============List sync summary results success")
}


func (n *Neutron) ResourceSync() {
	urlSuffix := "resourcesyncs"
	resp := n.List(n.headers, urlSuffix)
	log.Println(string(resp))
	log.Println("==============List resource sync success")
}

func (n *Neutron) ListAcConfig() {
	urlSuffix := "ac_config"
	resp := n.List(n.headers, urlSuffix)
	log.Println(string(resp))
	log.Println("==============List ac config success")
}

func (n *Neutron) CreateAcConfig() {
	urlSuffix := "ac_config"
	resp := n.Post(n.headers, urlSuffix, "{\"ac_config\": {}}")
	log.Println(string(resp))
	log.Println("==============Create ac config success")
}

// network quota

func (n *Neutron) ListQuotas()  {
	urlSuffix := "quotas"
	resp := n.Get(n.headers, urlSuffix)
	log.Println(string(resp))
}

func (n *Neutron) ListQuotaForProject()  {
    urlSuffix := fmt.Sprintf("quotas/%s", n.projectId)
    resp := n.Get(n.headers, urlSuffix)
    log.Println(string(resp))
}

func (n *Neutron) UpdateQuotaForProject() {
	urlSuffix := fmt.Sprintf("quotas/%s", n.projectId)
	opts := entity.NetworkQuotaMap{NetworkQuota: entity.NetworkQuota{
		Network: -1,
		Firewall: -1,
	}}
    reqBody := opts.ToRequestBody()
	resp := n.Put(n.headers, urlSuffix, reqBody)
	log.Println(string(resp))
}

func (n *Neutron) ListDefaultQuotaForProject()  {
	urlSuffix := fmt.Sprintf("quotas/%s/default", n.projectId)
	resp := n.Get(n.headers, urlSuffix)
	log.Println(string(resp))
}

func (n *Neutron) ResetQuotaForProject() {
	urlSuffix := fmt.Sprintf("quotas/%s", n.projectId)
	n.Delete(n.headers, urlSuffix)
}

func (n *Neutron) ShowQuotaDetailsForProject()  {
	//urlSuffix := fmt.Sprintf("quotas/%s/details", n.projectId)
	urlSuffix := "quotas/7e8babd4464e4c6da382a1a29d8da53a/details"
	resp := n.Get(n.headers, urlSuffix)
	log.Println(string(resp))
}

// service providers

func (n *Neutron) ListServiceProviders() {
	urlSuffix := "service-providers"
	resp := n.List(n.headers, urlSuffix)
	log.Println(string(resp))
}

// SNAT

func (n *Neutron) CreateSnat(opts *entity.Snat) entity.SnatMap {
	urlSuffix := consts.Snats
	createBody := opts.ToRequestBody()
	resp := n.Post(n.headers, urlSuffix, createBody)
	var snat entity.SnatMap
	_ = json.Unmarshal(resp, &snat)

	log.Printf("==============Create snat success %+v\n", snat)
	return snat
}

func (n *Neutron) ListSnats() entity.Snats {
	urlSuffix := fmt.Sprintf("snats?project_id=%s", n.projectId)
	resp := n.List(n.headers, urlSuffix)
	var snats entity.Snats
	_ = json.Unmarshal(resp, &snats)
	log.Printf("snats==%+v\n", snats)
	log.Println("==============List snats success, there had", len(snats.Ss))
	return snats
}

func (n *Neutron) DeleteSnat(snatId string) Output {
	outputObj := Output{ParametersMap: map[string]string{"network_id": snatId}}
	defer func() {
		if err := recover(); err != nil {
			log.Println("catch error：", err)
			outputObj.Success = false
			outputObj.Response = err
		}
	}()

	urlSuffix := fmt.Sprintf("snats/%s", snatId)
	outputObj.Success, outputObj.Response = n.Delete(n.headers, urlSuffix)
	return outputObj
}

func (n *Neutron) DeleteSnats() {
	snats := n.ListSnats()
	ch := n.MakeDeleteChannel(consts.Snat, len(snats.Ss))

	for _, snat := range snats.Ss {
		temp := snat
		go func() {
			ch <- n.DeleteSnat(temp.Id)
		}()
	}
	if len(ch) != cap(ch) {
		for len(ch) != cap(ch) {}
	}
	log.Println("Snats were deleted completely")
}

func (n *Neutron) CreateDnat(opts *entity.Dnat) entity.DnatMap {
	urlSuffix := consts.Dnats
	createBody := opts.ToRequestBody()
	resp := n.Post(n.headers, urlSuffix, createBody)
	var dnat entity.DnatMap
	_ = json.Unmarshal(resp, &dnat)

	log.Printf("==============Create dnat success %+v\n", dnat)
	return dnat
}

func (n *Neutron) ListDnats() entity.Dnats {
	urlSuffix := fmt.Sprintf("dnats?project_id=%s", n.projectId)
	resp := n.List(n.headers, urlSuffix)
	var dnats entity.Dnats
	_ = json.Unmarshal(resp, &dnats)
	log.Printf("dnats==%+v\n", dnats)
	log.Println("==============List snats success, there had", len(dnats.Ds))
	return dnats
}

func (n *Neutron) DeleteDnat(dnatId string) Output {
	outputObj := Output{ParametersMap: map[string]string{"network_id": dnatId}}
	defer func() {
		if err := recover(); err != nil {
			log.Println("catch error：", err)
			outputObj.Success = false
			outputObj.Response = err
		}
	}()

	urlSuffix := fmt.Sprintf("dnats/%s", dnatId)
	outputObj.Success, outputObj.Response = n.Delete(n.headers, urlSuffix)
	return outputObj
}

func (n *Neutron) DeleteDnats() {
	snats := n.ListDnats()
	ch := n.MakeDeleteChannel(consts.Dnat, len(snats.Ds))

	for _, snat := range snats.Ds {
		temp := snat
		go func() {
			ch <- n.DeleteDnat(temp.Id)
		}()
	}
	if len(ch) != cap(ch) {
		for len(ch) != cap(ch) {}
	}
	log.Println("Dnats were deleted completely")
}