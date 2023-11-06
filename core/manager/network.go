package manager

import (
	"encoding/json"
	"log"
	"os"
	"request_openstack/configs"
	"request_openstack/consts"
	"request_openstack/internal/entity"
	"runtime"
	"sync"
	"time"
)

// PrivateNetAccessToInternet 内网与Internet互访
func (m *Manager) PrivateNetAccessToInternet() {
	// 1. create vpc
	networkId, _, routerId := m.CreateVpc()

	// 2. create sg
	m.EnsureSgExist(configs.CONF.ProjectName)

	// 3. create instance
	instanceId := m.CreateInstanceHelper(networkId)

	// 4. set SNAT
    m.SetDefaultRouterGatewayHelper(routerId)

    // 5. create all allow acl, then associate router
    m.CreateFirewallAllAllowAndAssociateRouter(routerId)

    // 6. bind floating ip
    instancePortId, _ := m.GetInstancePort(instanceId)
    fipOpts := &entity.CreateFipOpts{FloatingNetworkID: configs.CONF.ExternalNetwork, PortID: instancePortId}
    m.CreateFloatingIP(fipOpts)
}

// InterconnectInSameVpc vms in the same vpc interconnected
func (m *Manager) InterconnectInSameVpc() {
	name := "interconnect_In_same_vpc"
	// 1. create vpc
	networkId, _, _ := m.CreateVpc()

	// 2. create sg
	m.EnsureSgExist(configs.CONF.ProjectName)

	// 3. create instance
	instanceOpts := entity.CreateInstanceOpts{
		FlavorRef: configs.CONF.FlavorId,
		ImageRef: configs.CONF.ImageId,
		Networks: []entity.ServerNet{{UUID: networkId}},
		AdminPass: "Wang.123",
		SecurityGroups: []entity.ServerSg{{Name: configs.CONF.ProjectName}},
		Name: name,
		Min: 2,
		Max: 2,
	}
	m.CreateMultipleInstances(instanceOpts)
}

// InterconnectInDifferentVpcFwEnabled vms in the different vpc interconnected
func (m *Manager) InterconnectInDifferentVpcFwEnabled() {
	name := "interconnect_In_different_vpc"
	// 1. create sg
	m.EnsureSgExist(configs.CONF.ProjectName)

	// 2. create local vpc
	networkId1, subnetId1, routerId1 := m.CreateVpc()

	// 3. set local SNAT
	m.SetDefaultRouterGatewayHelper(routerId1)

	// 4. create all allow acl, then associate router
	m.CreateFirewallAllAllowAndAssociateRouter(routerId1)

	// 5. create peer vpc
	networkId2, subnetId2, routerId2 := m.CreateVpc()

	// 6. set peer SNAT
	m.SetDefaultRouterGatewayHelper(routerId2)

	// 7. create all allow acl, then associate router
	m.CreateFirewallAllAllowAndAssociateRouter(routerId2)

	// 8. create local instance
	m.CreateInstanceHelper(networkId1)

	// 9. create peer instance
	m.CreateInstanceHelper(networkId2)

	// 10. create vpc connection
	vpcConnectionOpts := &entity.CreateVpcConnectionOpts{
		Name: name,
		LocalRouter: routerId1,
		PeerRouter: routerId2,
		LocalSubnets: []string{subnetId1},
		PeerSubnets: []string{subnetId2},
		Mode: 1,
		FwEnabled: true,
	}
    m.CreateVpcConnection(vpcConnectionOpts)
}


// InterconnectInDifferentVpcNoFw vms in the different vpc interconnected
func (m *Manager) InterconnectInDifferentVpcNoFw() {
	// 1. create sg
	m.EnsureSgExist(configs.CONF.ProjectName)

	// 2. create local vpc
	networkId1, subnetId1, routerId1 := m.CreateVpc()

	// 3. set local SNAT
	m.SetDefaultRouterGatewayHelper(routerId1)

	// 4. create peer vpc
	networkId2, subnetId2, routerId2 := m.CreateVpc()

	// 5. set peer SNAT
	m.SetDefaultRouterGatewayHelper(routerId2)

	// 6. create local instance
	m.CreateInstanceHelper(networkId1)

	// 7. create peer instance
	m.CreateInstanceHelper(networkId2)

	// 8. create vpc connection
	m.CreateVpcConnectionHelper(routerId1, routerId2, []string{subnetId1}, []string{subnetId2})
}

func (m *Manager) LoadbalancerPoolProtocolRR() {
	// 1. create lb vip network
	name := "lb_pool__rr"
	vipNetworkId := m.CreateNetworkHelper()
	vipSubnetId := m.CreateSubnetHelper(vipNetworkId)

	//2. create member instance
	instance1 := m.CreateInstanceHelper(vipNetworkId)
	instance2 := m.CreateInstanceHelper(vipNetworkId)

	// 3. create lb
	createLBOpts := entity.CreateLoadbalancerOpts{Name: name, VipSubnetID: vipSubnetId, }
	lbId := m.CreateLoadbalancer(createLBOpts)

	// 4. create listener
	createListenerOpts := entity.CreateListenerOpts{
		Name: name, LoadbalancerID: lbId, Protocol: entity.ProtocolTCP, ProtocolPort: 22}
	listener1 := m.CreateListener(createListenerOpts)

	// 5. create pool
	createPoolOpts := entity.CreatePoolOpts{
		Name: name, ListenerID: listener1,
		LBMethod: entity.LBMethodRoundRobin,
		Protocol: entity.ProtocolTCP}
	pool1 := m.CreatePool(createPoolOpts)

	// 6. create pool member
	_, ip1 := m.GetInstancePort(instance1)
	_, ip2 := m.GetInstancePort(instance2)
	createMemberOpts1 := entity.CreateMemberOpts{
		Name: name, Address: ip1, SubnetID: vipSubnetId, ProtocolPort: 22, Weight: 2}
	m.CreatePoolMember(pool1, createMemberOpts1)
	createMemberOpts2 := entity.CreateMemberOpts{
		Name: name, Address: ip2, SubnetID: vipSubnetId, ProtocolPort: 22, Weight: 1}
	m.CreatePoolMember(pool1, createMemberOpts2)

	// 7. create health monitor
	createHMOpts := entity.CreateHealthMonitorOpts{
		Name: name, PoolID: pool1, Type: entity.PING,
		MaxRetries: 5, Timeout: 30, Delay: 5}
	m.CreateHealthMonitor(createHMOpts)
}


func (m *Manager) LoadbalancerPoolProtocolLeastConnections() {
	// 1. create lb vip network
	name := "lb_pool__lc"
	vipNetworkId := m.CreateNetworkHelper()
	vipSubnetId := m.CreateSubnetHelper(vipNetworkId)

	//2. create member instance
	instance1 := m.CreateInstanceHelper(vipNetworkId)
	instance2 := m.CreateInstanceHelper(vipNetworkId)

	// 3. create lb
	createLBOpts := entity.CreateLoadbalancerOpts{Name: name, VipSubnetID: vipSubnetId}
	lbId := m.CreateLoadbalancer(createLBOpts)

	// 4. create listener
	connLimit := 5
	createListenerOpts := entity.CreateListenerOpts{
		Name: name, LoadbalancerID: lbId, Protocol: entity.ProtocolTCP,
		ProtocolPort: 822, ConnLimit: &connLimit}
	listener1 := m.CreateListener(createListenerOpts)

	// 5. create pool
	createPoolOpts := entity.CreatePoolOpts{
		Name: name, ListenerID: listener1,
		LBMethod: entity.LBMethodLeastConnections,
		Protocol: entity.ProtocolTCP}
	pool1 := m.CreatePool(createPoolOpts)

	// 6. create pool member
	_, ip1 := m.GetInstancePort(instance1)
	_, ip2 := m.GetInstancePort(instance2)
	createMemberOpts1 := entity.CreateMemberOpts{
		Name: name, Address: ip1, SubnetID: vipSubnetId, ProtocolPort: 22, Weight: 2}
	m.CreatePoolMember(pool1, createMemberOpts1)
	createMemberOpts2 := entity.CreateMemberOpts{
		Name: name, Address: ip2, SubnetID: vipSubnetId, ProtocolPort: 22, Weight: 1}
	m.CreatePoolMember(pool1, createMemberOpts2)

	// 7. create health monitor
	createHMOpts := entity.CreateHealthMonitorOpts{
		Name: name, PoolID: pool1, Type: entity.PING,
		MaxRetries: 5, Timeout: 30, Delay: 5}
	m.CreateHealthMonitor(createHMOpts)
}

// RDSAdmin_pt security group
// 名称：rds-security-group-odin
//
//出口：不受限制，IP协议：Any （dns需要 udp/tcp 53 ），端口范围：Any ，网段：0.0.0.0/0
//
//入口：IP协议：TCP 端口范围：8000、3300 : 3399、9104、9100、22 ，网段：10.50.0.0/0 、phpadmin server

// CreateRDSSecurityGroup 需要remote_ip_prefix
func (m *Manager) CreateRDSSecurityGroup() {
	name := "rds-security-group-odin"
	remoteIpPrefix := "10.240.20.0/22"
	opts := entity.CreateSecurityGroupOpts{Name: name}
	sgId := m.CreateSecurityGroupAndRules(&opts)

	ruleOpts8000 := &entity.CreateSecurityRuleOpts{
		Direction: consts.DirectionIngress, EtherType: consts.EtherTypeV4,
		PortRangeMax: 8000, PortRangeMin: 8000, RemoteIPPrefix: remoteIpPrefix,
		Protocol: consts.ProtocolTCP, SecGroupID: sgId}
	ruleOpts9100 := &entity.CreateSecurityRuleOpts{
		Direction: consts.DirectionIngress, EtherType: consts.EtherTypeV4,
		PortRangeMax: 9100, PortRangeMin: 9100, RemoteIPPrefix: remoteIpPrefix,
		Protocol: consts.ProtocolTCP, SecGroupID: sgId}
	ruleOpts9104 := &entity.CreateSecurityRuleOpts{
		Direction: consts.DirectionIngress, EtherType: consts.EtherTypeV4,
		PortRangeMax: 9104, PortRangeMin: 9104, RemoteIPPrefix: remoteIpPrefix,
		Protocol: consts.ProtocolTCP, SecGroupID: sgId}
	ruleOpts3300 := &entity.CreateSecurityRuleOpts{
		Direction: consts.DirectionIngress, EtherType: consts.EtherTypeV4,
		PortRangeMax: 3399, PortRangeMin: 3300, RemoteIPPrefix: remoteIpPrefix,
		Protocol: consts.ProtocolTCP, SecGroupID: sgId}
	ruleOpts22 := &entity.CreateSecurityRuleOpts{
		Direction: consts.DirectionIngress, EtherType: consts.EtherTypeV4,
		PortRangeMax: 22, PortRangeMin: 22, RemoteIPPrefix: remoteIpPrefix,
		Protocol: consts.ProtocolTCP, SecGroupID: sgId}
    m.CreateSecurityGroupRule(ruleOpts22.ToRequestBody())
    m.CreateSecurityGroupRule(ruleOpts8000.ToRequestBody())
    m.CreateSecurityGroupRule(ruleOpts9100.ToRequestBody())
    m.CreateSecurityGroupRule(ruleOpts9104.ToRequestBody())
    m.CreateSecurityGroupRule(ruleOpts3300.ToRequestBody())
}

func (m *Manager) FipQosLimit() {
	netId, _, routerId := m.CreateVpc()
	m.SetDefaultRouterGatewayHelper(routerId)
	instanceId := m.CreateInstanceHelper(netId)
	instancePortId, _ := m.GetInstancePort(instanceId)
	fipId := m.CreateFloatingipHelper()
	m.UpdateFloatingIpWithPort(fipId, instancePortId)
	fipPort := m.GetFloatingipPort(fipId)
	qosId := m.CreateQosPolicyHelper()
	m.UpdatePortWithQos(fipPort.Id, qosId)
}

// FipPortForwarding fip port forwarding
func (m *Manager) FipPortForwarding() {
	netId, _, routerId := m.CreateVpc()
	m.SetDefaultRouterGatewayHelper(routerId)
	instanceId := m.CreateInstanceHelper(netId)
	fipId := m.CreateFloatingipHelper()
	m.CreatePortForwardingHelper(fipId, instanceId)
}

type fipAssoc struct {
	FipId                      string       `json:"fip_id"`
	FipAssocPort               string       `json:"fipAssocPort"`
	FixedIpAddress             string       `json:"fixed_ip_address"`
	QosPolicyId                string       `json:"qos_policy_id"`
	FipPort                    string       `json:"fip_port"`
	FloatingNetworkID          string       `json:"floating_network_id"`
	FloatingIpAddr             string       `json:"floating_ip_address"`
	entity.PortForwardings                  `json:"port_forwardings"`
}

type routerAssoc struct {
	RouterId                  string      `json:"router_id"`
	entity.GatewayInfo                    `json:"external_gateway_info"`
}

type L3RelatedResource struct {
	Routers              []routerAssoc      `json:"routers"`
    Fips                 []fipAssoc         `json:"fips"`
}

func (m *Manager) generateL3RelatedResObjs() L3RelatedResource {
	var lrr = L3RelatedResource{}

	routers := m.ListRouters()
	var routerAssocs = make([]routerAssoc, 0)
	for _, router := range routers.Rs {
		routerAssoc := routerAssoc{
			RouterId: router.Id,
			GatewayInfo: router.GatewayInfo,
		}
		routerAssocs = append(routerAssocs, routerAssoc)
	}
	lrr.Routers = routerAssocs

    var relatedFips = make([]fipAssoc, 0)
	fips := m.ListFIPs()
	for _, fip := range fips.Fs {
		pfs := m.ListPortForwarding(fip.Id)
		fipPort := m.GetFloatingipPort(fip.Id)
		var qosPolicyId string
		if fipPort.QosPolicyId != nil {
			qosPolicyId = fipPort.QosPolicyId.(string)
		} else {
			qosPolicyId = ""
		}

		relatedFips = append(relatedFips, fipAssoc{
			FipId: fip.Id,
			FipAssocPort: fip.PortId,
			PortForwardings: pfs,
			QosPolicyId: qosPolicyId,
			FloatingNetworkID: fip.FloatingNetworkId,
			FipPort: fipPort.Id,
			FloatingIpAddr: fip.FloatingIpAddress,
			FixedIpAddress: fip.FixedIpAddress,
		})
	}
	lrr.Fips = relatedFips
    log.Printf("==============Generate l3 related resource %+v", lrr)
	return lrr
}

func (m *Manager) exportToJsonFile(lrr L3RelatedResource) {
    currentPath, _ := os.Getwd()
    if runtime.GOOS == "windows" {
		currentPath += "\\"
	} else if runtime.GOOS == "linux" {
		currentPath += "/"
	}
	fileName := currentPath + time.Now().Format("2006-01-02_15-04-05-1") + "_record.json"
	var file, err = os.Create(fileName)
	defer file.Close()
	if err != nil {
		log.Fatalf("Failed to create file %s, %v", fileName, err)
	}
	data, _ := json.Marshal(lrr)
	file.Write(data)
	log.Println("==============Export to json file success", fileName)
}

func (m *Manager) DeleteL3RelatedRes(lrr L3RelatedResource) {
	for _, fip := range lrr.Fips {
		for _, pf := range fip.PortForwardings.Pfs {
			m.DeletePortForwarding(fip.FipId, pf.Id)
		}

		if len(fip.FipAssocPort) != 0 {
			m.FloatingIpDisassociatePort(fip.FipId)
		}

		m.UpdatePortWithNoQos(fip.FipPort)
	}

	for _, router := range lrr.Routers {
		m.ClearRouterGateway(router.RouterId, router.NetworkID)
	}
	log.Println("==============The L3 related resources were deleted success")
}

func (m *Manager) getRecordFromJsonFile(jsonFile string) L3RelatedResource {
	data, err := os.ReadFile(jsonFile)
	if err != nil {
		log.Fatalln("Failed to read json file", err)
	}

	var lrr L3RelatedResource
	err = json.Unmarshal(data, &lrr)
	if err != nil {
		log.Fatalln("Failed to unmarshal json file", err)
	}
	return lrr
}

func (m *Manager) setRouterGateway(router routerAssoc, wg *sync.WaitGroup) {
	opts := entity.UpdateRouterOpts{
		GatewayInfo: &router.GatewayInfo,
	}
	m.UpdateRouter(router.RouterId, &opts)
	wg.Done()
}

func (m *Manager) processFip(fip fipAssoc, wg *sync.WaitGroup) {
	if len(fip.FipAssocPort) != 0 {
		m.UpdateFloatingIpWithPortIpAddress(fip.FipId, fip.FipAssocPort, fip.FixedIpAddress)
	}

	if len(fip.QosPolicyId) != 0 {
		m.UpdatePortWithQos(fip.FipPort, fip.QosPolicyId)
	}

	for _, pf := range fip.PortForwardings.Pfs {
		pfOpts := entity.CreatePortForwardingOpts{
			Protocol: pf.Protocol,
			InternalIPAddress: pf.InternalIpAddress,
			InternalPort: pf.InternalPort,
			InternalPortID: pf.InternalPortId,
			ExternalPort: pf.ExternalPort,
		}
		m.CreatePortForwarding(fip.FipId, &pfOpts)
	}
	wg.Done()
}

func (m *Manager) RunRecoverL3Res(jsonFile string) {
	lrr := m.getRecordFromJsonFile(jsonFile)
	//lrr := m.generateL3RelatedResObjs()

	// set router gateway
	var wg sync.WaitGroup
	for _, router := range lrr.Routers {
		wg.Add(1)
		go m.setRouterGateway(router, &wg)
	}
	wg.Wait()

	// create fip, fip associate port, fip port set qos, fip set port forwarding
    for _, fip := range lrr.Fips {
    	wg.Add(1)
    	go m.processFip(fip, &wg)
	}
	wg.Wait()
}

func (m *Manager) RunGenerateRecord() {
	lrr := m.generateL3RelatedResObjs()
	m.exportToJsonFile(lrr)
}

func (m *Manager) RunDeleteRes()  {
	lrr := m.generateL3RelatedResObjs()
	m.exportToJsonFile(lrr)
	m.DeleteL3RelatedRes(lrr)
}
