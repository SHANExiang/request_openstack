package manager

import (
	"fmt"
	"github.com/valyala/fasthttp"
	"log"
	"math/rand"
	"request_openstack/configs"
	"request_openstack/consts"
	"request_openstack/internal"
	"request_openstack/internal/entity"
	"time"
)

var (
	neutronUri = fmt.Sprintf("%d/v2.0/", consts.NeutronPort)
	novaUri = fmt.Sprintf("%d/v2.1/", consts.NovaPort)
	cinderUri = fmt.Sprintf("%d/v3/", consts.CinderPort)
	defaultClient *fasthttp.Client
	DefaultName = "default"
)

func init() {
	defaultClient = internal.NewClient()
}


type Manager struct {
	*internal.Keystone
	*internal.Nova
	*internal.Neutron
	*internal.Cinder
	*internal.Glance
	*internal.Octavia
	*internal.SDN
}

func NewAdminManager() *Manager {
	keystone := internal.NewKeystone(defaultClient)
	token := keystone.GetToken(consts.ADMIN, consts.ADMIN, configs.CONF.AdminPassword)
	keystone.SetHeader(consts.AuthToken, token)
	projectId := keystone.GetProjectId(configs.CONF.ProjectName)
	if len(projectId) == 0 {
		log.Fatalln("==============The user name not exist!!!\n")
	}
	adminProjectId := keystone.GetProjectId(consts.ADMIN)
	return &Manager{
		Keystone: keystone,
		Nova: internal.NewNova(
			internal.WithToken(token),
			internal.WithProjectId(projectId),
			internal.WithRequest(novaUri, defaultClient),
			internal.WithSnowFlake()),
		Neutron: internal.NewNeutron(
			internal.WithToken(token),
			internal.WithProjectId(projectId),
			internal.WithRequest(neutronUri, defaultClient),
			internal.WithSnowFlake(),
			internal.WithIsAdmin(true)),
		Cinder: internal.NewCinder(
			internal.WithAdminProjectId(adminProjectId),
			internal.WithToken(token),
			internal.WithProjectId(projectId),
			internal.WithRequest(cinderUri, defaultClient)),
		Glance:   internal.NewGlance(token, projectId, defaultClient),
		Octavia:  internal.NewLB(token, defaultClient),
	}
}

func NewManager() *Manager {
	keystone := internal.NewKeystone(defaultClient)
	adminToken := keystone.GetToken(consts.ADMIN, consts.ADMIN, configs.CONF.AdminPassword)
	keystone.SetHeader(consts.AuthToken, adminToken)
	projectId := keystone.MakeSureProjectExist()
	token := keystone.GetToken(configs.CONF.ProjectName, configs.CONF.UserName, configs.CONF.UserPassword)
	keystone.SetHeader(consts.AuthToken, token)
	manager := &Manager{
		Keystone: keystone,
		Nova: internal.NewNova(
			internal.WithToken(token),
			internal.WithProjectId(projectId),
			internal.WithRequest(novaUri, defaultClient),
			internal.WithSnowFlake()),
		Neutron: internal.NewNeutron(
			internal.WithToken(token),
			internal.WithProjectId(projectId),
			internal.WithRequest(neutronUri, defaultClient),
			internal.WithSnowFlake(),
			internal.WithIsAdmin(false)),
		Cinder: internal.NewCinder(
			internal.WithToken(token),
			internal.WithProjectId(projectId),
			internal.WithRequest(cinderUri, defaultClient)),
		Glance:   internal.NewGlance(token, projectId, defaultClient),
		Octavia:  internal.NewLB(token, defaultClient),
		//SDN:      internal.NewSDN(),
	}
	return manager
}


func (m *Manager) CreateNetworkHelper() string {
	netOpts := &entity.CreateNetworkOpts{Name: DefaultName, Description: DefaultName}
	netId := m.CreateNetwork(netOpts)
	return netId
}

func (m *Manager) CreateExternalNetwork() (string, string) {
	netOpts := &entity.CreateNetworkOpts{Name: "ext_net", RouterExternal: true}
	netId := m.CreateNetwork(netOpts)
	subnetId := m.CreateSubnetHelper(netId)
	return netId, subnetId
}

func (m *Manager) CreateSubnetHelper(netId string) string {
	rand.Seed(time.Now().UnixNano())
	randomNum := rand.Intn(200)
	cidr := fmt.Sprintf("192.%d.%d.0/24", randomNum, randomNum)
	gatewayIp1 := fmt.Sprintf("192.%d.%d.1", randomNum, randomNum)
	subnetOpts := &entity.CreateSubnetOpts{
		NetworkID: netId,
		CIDR: cidr,
		IPVersion: 4,
		GatewayIP: &gatewayIp1,
		DNSNameservers: []string{"114.114.114.114"},
	}
	subnetId := m.CreateSubnet(subnetOpts)
	return subnetId
}

func (m *Manager) CreatePortHelper(netId, subnetId string) string {
	fixedIp1 := entity.FixedIP{SubnetId: subnetId}
	fixedIp2 := entity.FixedIP{SubnetId: subnetId}
    opts := &entity.CreatePortOpts{
    	FixedIp: []entity.FixedIP{fixedIp1, fixedIp2},
        NetworkId: netId,
	}
    return m.CreatePort(opts)
}

func (m *Manager) CreateRouterHelper() string {
	routerOpts := &entity.CreateRouterOpts{
		Name: DefaultName, Description: DefaultName,
	}
	routerId := m.CreateRouter(routerOpts)
	return routerId
}

func (m *Manager) SetRouterGatewayHelper(routerId, externalNetId string)  {
	updateRouterOpts := &entity.UpdateRouterOpts{
		GatewayInfo: &entity.GatewayInfo{
			NetworkID: externalNetId, QosPolicyId: "fe413250-e243-4694-b0b3-182731cd6f34"}}
	m.UpdateRouter(routerId, updateRouterOpts)
}

func (m *Manager) SetRouterGatewaySpecifyIPHelper(routerId, externalNetId, subnetId, fixedIP string)  {
	fixedIp := entity.ExternalFixedIP{
		IPAddress: fixedIP,
		SubnetID: subnetId,
	}
	updateRouterOpts := &entity.UpdateRouterOpts{
		GatewayInfo: &entity.GatewayInfo{
			NetworkID: externalNetId,
			ExternalFixedIPs: []entity.ExternalFixedIP{fixedIp},
		}}
	m.UpdateRouter(routerId, updateRouterOpts)
}

func (m *Manager) SetDefaultRouterGatewayHelper(routerId string) {
	updateRouterOpts := &entity.UpdateRouterOpts{
		GatewayInfo: &entity.GatewayInfo{
			NetworkID: configs.CONF.ExternalNetwork}}
	m.UpdateRouter(routerId, updateRouterOpts)
}

func (m *Manager) AddRouterInterfaceHelper(routerId, subnetId string) {
	addInterfaceOpts := &entity.AddRouterInterfaceOpts{
		SubnetID: subnetId, RouterId: routerId,
	}
	m.AddRouterInterface(addInterfaceOpts)
}

func (m *Manager) CreateInstanceHelper(netId string) string {
	m.EnsureSgExist(DefaultName)
	instanceOpts := entity.CreateInstanceOpts{
		FlavorRef:      configs.CONF.FlavorId,
		ImageRef:       configs.CONF.ImageId,
		Networks:       []entity.ServerNet{{UUID: netId}},
		AdminPass:      "Wang.123",
		SecurityGroups: []entity.ServerSg{{Name: DefaultName}},
		Name:           DefaultName,
		BlockDeviceMappingV2: []entity.BlockDeviceMapping{{
			BootIndex: 0, Uuid: configs.CONF.ImageId, SourceType: "image",
			DestinationType: "volume", VolumeSize: 20, DeleteOnTermination: true,
		}},
	}
	instanceId := m.CreateInstance(&instanceOpts)
	return instanceId
}

func (m *Manager) CreateInstanceByVolumeHelper(netId, volumeId string) string {
	m.EnsureSgExist(DefaultName)
	instanceOpts := entity.CreateInstanceOpts{
		FlavorRef:      configs.CONF.FlavorId,
		Networks:       []entity.ServerNet{{UUID: netId}},
		AdminPass:      "Wang.123",
		SecurityGroups: []entity.ServerSg{{Name: DefaultName}},
		Name:           DefaultName,
		BlockDeviceMappingV2: []entity.BlockDeviceMapping{{
			BootIndex: 0, Uuid: volumeId, SourceType: "volume",
			DestinationType: "volume", VolumeSize: 10, DeleteOnTermination: true,
		}},
	}
	instanceId := m.CreateInstance(&instanceOpts)
	return instanceId
}

func (m *Manager) CreateInstanceWithPortHelper(portId string) string {
	m.EnsureSgExist(DefaultName)
	instanceOpts := entity.CreateInstanceOpts{
		FlavorRef:      configs.CONF.FlavorId,
		Networks:       []entity.ServerNet{{Port: portId}},
		AdminPass:      "Wang.123",
		SecurityGroups: []entity.ServerSg{{Name: DefaultName}},
		Name:           DefaultName,
		BlockDeviceMappingV2: []entity.BlockDeviceMapping{{
			BootIndex: 0, Uuid: configs.CONF.ImageId, SourceType: "image",
			DestinationType: "volume", VolumeSize: 10, DeleteOnTermination: true,
		}},
	}
	instanceId := m.CreateInstance(&instanceOpts)
	return instanceId
}

func (m *Manager) CreateQosPolicyHelper() string {
	qosId := m.CreateQos()
	m.CreateBandwidthLimitRuleIngress(qosId)
	m.CreateBandwidthLimitRuleEgress(qosId)
	return qosId
}

func (m *Manager) CreateVpcConnectionHelper(localRouter, peerRouter string, localSubnets, peerSubnets []string) string {
	vpcConnectionOpts := &entity.CreateVpcConnectionOpts{
		Name: DefaultName,
		LocalRouter: localRouter,
		PeerRouter: peerRouter,
		LocalSubnets: localSubnets,
		PeerSubnets: peerSubnets,
	}
	return m.CreateVpcConnection(vpcConnectionOpts)
}

func (m *Manager) CreateVpcConnectionWithCidrHelper(localRouter, peerRouter string, localCidrs, peerCidrs []string) string {
	vpcConnectionOpts := &entity.CreateVpcConnectionOpts{
		Name: DefaultName,
		LocalRouter: localRouter,
		PeerRouter: peerRouter,
		LocalCidrs: localCidrs,
		PeerCidrs: peerCidrs,
	}
	return m.CreateVpcConnection(vpcConnectionOpts)
}

func (m *Manager) UpdateVpcConnectionWithCidrHelper(vpcConnId string, localCidrs, peerCidrs []string) string {
	vpcConnectionOpts := &entity.UpdateVpcConnectionOpts{
		LocalCidrs: localCidrs,
		PeerCidrs: peerCidrs,
	}
	return m.UpdateVpcConnection(vpcConnId, vpcConnectionOpts)
}

func (m *Manager) CreateFloatingipHelper() string {
	opts := &entity.CreateFipOpts{FloatingNetworkID: configs.CONF.ExternalNetwork}
	return m.CreateFloatingIP(opts)
}

func (m *Manager) CreateFloatingipWithPortHelper(portId string) string {
	opts := &entity.CreateFipOpts{
		FloatingNetworkID: configs.CONF.ExternalNetwork, PortID: portId}
	return m.CreateFloatingIP(opts)
}

func (m *Manager) CreateSnatHelper(routerId, subnetId, natIp string) {
	subnet := m.GetSubnet(subnetId)
	opts := &entity.Snat{SnatNetworkId: configs.CONF.ExternalNetwork,
		OriginalCidrs: []string{subnet.Cidr}, TenantId: "48ad435f0e8c44598d3236acdbb9ca47",
		RouterId: routerId, SnatIpAddress: natIp}
	m.CreateSnat(opts)
}

func (m *Manager) CreateDnatHelper(floatingipId, floatingipAddr, portId string) {
	rand.Seed(123)
	portIP := m.GetPortIP(portId)
	opts := &entity.Dnat{
		FloatingipId: floatingipId,
		PortId: portId,
		TenantId: "48ad435f0e8c44598d3236acdbb9ca47",
		FixedIpAddress: portIP,
		FloatingIpAddress: floatingipAddr,
		Protocol: consts.ProtocolTCP,
		FloatingIpPort: rand.Intn(65535),
		FixedIpPort: 22,
	}
	m.CreateDnat(opts)
}

func (m *Manager) CreatePortForwardingHelper(fipId string, instanceId string) string {
	internalPort, internalIpAddr := m.GetInstancePort(instanceId)
	opts := entity.CreatePortForwardingOpts{
		InternalPortID: internalPort,
		InternalIPAddress: internalIpAddr,
		InternalPort: 10000,
		ExternalPort: 10001,
		Protocol: consts.ProtocolTCP,
	}
	return m.CreatePortForwarding(fipId, &opts)
}

// CleanProjectAndUser delete project and user
func (m *Manager) CleanProjectAndUser() {
	delete(m.Keystone.Headers, consts.AuthToken)
	token := m.GetToken(consts.ADMIN, consts.ADMIN, configs.CONF.AdminPassword)
	m.Keystone.SetHeader(consts.AuthToken, token)
	m.DeleteUserByName(configs.CONF.UserName)
	m.DeleteProjectByName(configs.CONF.ProjectName)
}

func (m *Manager) CleanGlanceResources() {
	m.DeleteImage("91f28959-3f5d-4993-a793-44eb31ca3d8c")
}

func (m *Manager) CreateVpc() (string, string, string) {
	// 1. create network
	netId := m.CreateNetworkHelper()

	// 2. create subnet
	subnetId := m.CreateSubnetHelper(netId)

	// 3. create router
	routerId := m.CreateRouterHelper()

	// 4. router add subnet
    m.AddRouterInterfaceHelper(routerId, subnetId)
	return netId, subnetId, routerId
}

func (m *Manager) CreateFirewallRuleHelper(protocol string, action string) string {
	allowAnyRuleOpts := &entity.CreateFirewallRuleOpts{Name: DefaultName, Protocol: protocol, Action: action}
	return m.CreateFirewallRuleV1(allowAnyRuleOpts)
}

func (m *Manager) CreateFirewallPolicy() string {
	policyOpts := &entity.CreateFirewallPolicyOpts{Name: DefaultName}
	return m.CreateFirewallPolicyV1(policyOpts)
}

func (m *Manager) CreateFirewallHelper(policyId string) string {
	firewallOpts := &entity.CreateFirewallOpts{Name: DefaultName, PolicyID: policyId, RouterIDs: []string{}}
	return m.CreateFirewallV1(firewallOpts)
}

func (m *Manager) FirewallAssociateRoutersHelper(firewallId string, routerIds []string)  {
	updateOpts := &entity.UpdateFirewallOpts{RouterIDs: routerIds}
	m.UpdateFirewallV1(firewallId, updateOpts)
}

func (m *Manager) CreateFirewallAllAllowAndAssociateRouter(routerId string) string {
    ruleId := m.CreateFirewallRuleHelper(consts.ProtocolAny, consts.ActionAllow)
    firewallPolicyId := m.CreateFirewallPolicy()
    m.UpdateFirewallPolicyInsertRuleV1(firewallPolicyId, ruleId)
    firewallId := m.CreateFirewallHelper(firewallPolicyId)
    m.FirewallAssociateRoutersHelper(firewallId, []string{routerId})
    return firewallId
}

func (m *Manager) CreateFirewallAllDenyAndAssociateRouter(routerId string) string {
	ruleId := m.CreateFirewallRuleHelper(consts.ProtocolAny, consts.ActionDeny)
	firewallPolicyId := m.CreateFirewallPolicy()
	m.UpdateFirewallPolicyInsertRuleV1(firewallPolicyId, ruleId)
	firewallId := m.CreateFirewallHelper(firewallPolicyId)
	m.FirewallAssociateRoutersHelper(firewallId, []string{routerId})
	return firewallId
}

func (m *Manager) CreateFirewallRuleAllowSSH() string {
	allowAnyRuleOpts := &entity.CreateFirewallRuleOpts{Name: DefaultName, Protocol: consts.ProtocolTCP,
		Action: consts.ActionAllow, SourcePort: "22", DestinationPort: "22"}
	ruleId := m.CreateFirewallRuleV1(allowAnyRuleOpts)
	return ruleId
}

func (m *Manager) CreateFirewallRuleDenySnat(sourceIpAddress, destIpAddress string) string {
	opts := &entity.CreateFirewallRuleOpts{
		Name: DefaultName,
		Action: consts.ActionDeny,
		Protocol: consts.ProtocolAny,
		SourceIPAddress: sourceIpAddress,
		DestinationIPAddress: destIpAddress,
		IPVersion: 4,
	}
	ruleId := m.CreateFirewallRuleV1(opts)
	return ruleId
}

func (m *Manager) CreateFirewallRuleAllowSnat(snatIp string) string {
	opts := &entity.CreateFirewallRuleOpts{
		Name: DefaultName,
		Action: consts.ActionAllow,
		Protocol: consts.ProtocolAny,
		SourceIPAddress: snatIp,
		IPVersion: 4,
	}
	ruleId := m.CreateFirewallRuleV1(opts)
	return ruleId
}

func (m *Manager) CreateFirewallRuleAllowDnat(vpcCidr string) string {
	opts := &entity.CreateFirewallRuleOpts{
		Name: DefaultName,
		Action: consts.ActionAllow,
		Protocol: consts.ProtocolAny,
		DestinationIPAddress: vpcCidr,
		IPVersion: 4,
	}
	ruleId := m.CreateFirewallRuleV1(opts)
	return ruleId
}

func (m *Manager) CreateLoadbalancerHelper(vipSubnetId string) string {
	createLBOpts := entity.CreateLoadbalancerOpts{VipSubnetID: vipSubnetId}
	return m.CreateLoadbalancer(createLBOpts)
}

// CreateVpnIpsecConnection the vpcs of two cluster connect with vpn
func (m *Manager) CreateVpnIpsecConnection(routerId, subnetId, peerCidr, peerAddress string) {
	localVpnServiceId := m.CreateVpnService(routerId)

	localEGId := m.CreateLocalEndpointGroup(subnetId)

	peerEGId := m.CreatePeerEndpointGroup(peerCidr)

	ikePolicy := m.CreateIkePolicy()
	ipsecPolicy := m.CreateIpsecPolicy()

	m.CreateIpsecConnection(localVpnServiceId, ikePolicy, ipsecPolicy, peerEGId, localEGId, peerAddress)
}

func (m *Manager) VpnIpsecConnectionDelete() {
	m.DeleteIpsecConnections()
	m.DeleteIkePolicies()
	m.DeleteIpsecPolicies()
	m.DeleteEndpointGroups()
	m.DeleteVpnServices()
	//m.DeleteRouters()
	//m.DeleteNetworks()
}

func (m *Manager) SnapshotToImage() {
	volume := m.CreateVolume()
	snapshot := m.CreateSnapshot(volume)
	TempVolume := m.CreateVolumeBySnapshot(snapshot)
	image := m.UploadToImage(TempVolume)
    imageDetail := m.GetImage(image)
    log.Println(imageDetail)
}

func (m *Manager) ImageShared() {
	m.SetImageVisibilityProperty("91f28959-3f5d-4993-a793-44eb31ca3d8c", "shared")
	m.CreateImageMember("91f28959-3f5d-4993-a793-44eb31ca3d8c", "375d082324a34f5f957d3be428da4506")
	m.SetImageMemberStatus("91f28959-3f5d-4993-a793-44eb31ca3d8c", "375d082324a34f5f957d3be428da4506", "accepted")
}

func (m *Manager) Compensate() {
	policyOpts := &entity.CreateFirewallPolicyOpts{Name: "dx_test2", Rules: []string{}}
	m.CreateFirewallPolicyV1(policyOpts)
}

func (m *Manager) Cre()  {

}
