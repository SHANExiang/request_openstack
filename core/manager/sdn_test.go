package manager

import (
    "fmt"
    "reflect"
    "request_openstack/configs"
    "request_openstack/consts"
    "request_openstack/internal/entity"
    "testing"
)

var (
    manager *Manager
)


func init() {
    configs.Viper()
    manager = NewManager()
}

// TestCreateQosPolicy 1. test qos
func TestCreateQosPolicy(t *testing.T) {
    qosId := manager.CreateQosAndRule()
    qos := manager.GetQos(qosId)

    if len(qos.Rules) != 3 {
        t.Fatal("Create qos policy and rule failed")
    }
    t.Cleanup(func() {
       manager.DeleteQos(qosId)
    })
}

// TestCreateNetwork 2. test network
func TestCreateNetwork(t *testing.T) {
    netId := manager.CreateNetworkHelper()
    network := manager.GetNetwork(netId)
    if network.Name != DefaultName + "_" + consts.NETWORK || network.Description != DefaultName {
        t.Fatal("Create network failed")
    }
    t.Cleanup(func() {
        manager.DeleteNetwork(netId)
    })
}

// TestUpdateNetwork 3. test update network
func TestUpdateNetwork(t *testing.T) {
    netId := manager.CreateNetworkHelper()
    updateName := "test_update_network"
    updateBody := fmt.Sprintf("{\"network\": {\"name\": \"%+v\"}}", updateName)
    manager.UpdateNetwork(netId, updateBody)

    network := manager.GetNetwork(netId)
    if network.Name != updateName {
        t.Fatal("Update network failed")
    }

    t.Cleanup(func() {
        manager.DeleteNetwork(netId)
    })
}

// TestCreateSubnet 4. test subnet
func TestCreateSubnet(t *testing.T) {
    netId := manager.CreateNetworkHelper()

    gatewayIp1 := "11.11.11.1"
    subnetOpts := &entity.CreateSubnetOpts{
        NetworkID: netId,
        CIDR: "11.11.11.0/24",
        IPVersion: 4,
        GatewayIP: &gatewayIp1,
        DNSNameservers: []string{"114.114.114.114"},
    }
    subnetId := manager.CreateSubnet(subnetOpts)
    subnet := manager.GetSubnet(subnetId)
    if subnet.Subnet.Cidr != "11.11.11.0/24" || subnet.Subnet.GatewayIp != "11.11.11.1" ||
        subnet.Subnet.AllocationPools[0].Start != "11.11.11.2" ||
        subnet.Subnet.AllocationPools[0].End != "11.11.11.254" ||
        !reflect.DeepEqual(subnet.Subnet.DnsNameservers[0], "114.114.114.114") {
        t.Fatal("Create subnet failed")
    }

    gatewayIp2 := "22.22.22.1"
    subnetOpts2 := &entity.CreateSubnetOpts{
        NetworkID: netId,
        CIDR: "22.22.22.0/24",
        IPVersion: 4,
        GatewayIP: &gatewayIp2,
        DNSNameservers: []string{"114.114.114.114"},
    }
    subnetId2 := manager.CreateSubnet(subnetOpts2)
    subnet2 := manager.GetSubnet(subnetId2)
    if subnet2.Subnet.Cidr != "22.22.22.0/24" || subnet2.Subnet.GatewayIp != "22.22.22.1" ||
        subnet2.Subnet.AllocationPools[0].Start != "22.22.22.2" ||
        subnet2.Subnet.AllocationPools[0].End != "22.22.22.254" ||
        !reflect.DeepEqual(subnet2.Subnet.DnsNameservers[0], "114.114.114.114") {
        t.Fatal("Create subnet failed")
    }
    t.Cleanup(func() {
        manager.DeleteNetwork(netId)
    })
}

// TestCreateRouter 5. test router
func TestCreateRouter(t *testing.T) {
    routerId := manager.CreateRouterHelper()
    manager.SetDefaultRouterGatewayHelper(routerId)
    router := manager.GetRouter(routerId)
    if router.Name != DefaultName + "_" + consts.ROUTER || router.Description != DefaultName ||
        router.ExternalGatewayInfo.NetworkId != configs.CONF.ExternalNetwork {
        t.Fatal("Create router failed")
    }
    t.Cleanup(func() {
        manager.DeleteRouter(routerId)
    })
}

// TestAddRemoveRouterInterface 6. test router interface
func TestAddRemoveRouterInterface(t *testing.T) {
    routerId := manager.CreateRouterHelper()
    manager.SetDefaultRouterGatewayHelper(routerId)
    netId := manager.CreateNetworkHelper()

    gatewayIp1 := "100.100.100.1"
    subnetOpts := &entity.CreateSubnetOpts{
        NetworkID: netId,
        CIDR: "100.100.100.0/24",
        IPVersion: 4,
        GatewayIP: &gatewayIp1,
        DNSNameservers: []string{"114.114.114.114"},
    }
    subnetId := manager.CreateSubnet(subnetOpts)

    addInterfaceOpts := &entity.AddRouterInterfaceOpts{
        SubnetID: subnetId, RouterId: routerId,
    }
    manager.AddRouterInterface(addInterfaceOpts)
    port := manager.GetPortByDevice(routerId, consts.NETWORKROUTERINTERFACE)
    if port == nil || port.DeviceId != routerId ||
        port.DeviceOwner != consts.NETWORKROUTERINTERFACE ||
        port.FixedIps[0].IpAddress != gatewayIp1 {
        t.Fatal("Add router interface failed")
    }

    manager.RemoveRouterInterface(routerId, subnetId)
    port = manager.GetPortByDevice(routerId, consts.NETWORKROUTERINTERFACE)
    if port != nil {
        t.Fatal("Remove router interface failed")
    }
    t.Cleanup(func() {
        manager.DeleteRouter(routerId)
        manager.DeleteNetwork(netId)
    })
}

// TestCreateFip 7. test floating ip
func TestCreateFip(t *testing.T) {
    routerId := manager.CreateRouterHelper()
    manager.SetDefaultRouterGatewayHelper(routerId)
    fipOpts := &entity.CreateFipOpts{
        FloatingNetworkID: configs.CONF.ExternalNetwork,
    }
    fipId := manager.CreateFloatingIP(fipOpts)
    port := manager.GetPortByDevice(fipId, consts.NETWORKFLOATINGIP)
    if port == nil || port.DeviceId != fipId || port.DeviceOwner != consts.NETWORKFLOATINGIP {
        t.Fatal("Create floating ip failed")
    }

    netId := manager.CreateNetworkHelper()
    subnetId := manager.CreateSubnetHelper(netId)
    manager.AddRouterInterfaceHelper(routerId, subnetId)
    instanceId := manager.CreateInstanceHelper(netId)
    instancePortId, _ := manager.GetInstancePort(instanceId)

    manager.UpdateFloatingIpWithPort(fipId, instancePortId)
    fip := manager.GetFIP(fipId)
    if fip.PortId == "" {
        t.Fatal("Floating ip associate instance failed")
    }

    qosId := manager.CreateQosPolicyHelper()
    manager.UpdatePortWithQos(port.Id, qosId)
    fipPort := manager.GetPort(port.Id)
    if fipPort.QosPolicyId != qosId {
        t.Fatal("Floating ip associate qos policy failed")
    }

    t.Cleanup(func() {
        manager.DeleteFIP(fipId)
        manager.DeleteInstance(instanceId)
        manager.RemoveRouterInterface(routerId, subnetId)
        manager.DeleteRouter(routerId)
        manager.DeleteNetwork(netId)
        manager.DeleteQos(qosId)
    })
}

// TestCreateInstance 8. test instance`
func TestCreateInstance(t *testing.T) {
    netId := manager.CreateNetworkHelper()
    manager.CreateSubnetHelper(netId)
    instanceId := manager.CreateInstanceHelper(netId)
    instancePortId, _ := manager.GetInstancePort(instanceId)
    qosId := manager.CreateQosPolicyHelper()
    manager.UpdatePortWithQos(instancePortId, qosId)
    fipPort := manager.GetPort(instancePortId)
    if fipPort.QosPolicyId != qosId {
        t.Fatal("Floating ip associate qos policy failed")
    }
    t.Cleanup(func() {
        manager.DeleteInstance(instanceId)
        manager.DeleteNetwork(netId)
        manager.DeleteQos(qosId)
    })
}

// TestCreateVpcConnection 9. test vpc connection
func TestCreateVpcConnection(t *testing.T) {
    netId1 := manager.CreateNetworkHelper()
    subnetId1 := manager.CreateSubnetHelper(netId1)
    routerId1 := manager.CreateRouterHelper()
    manager.SetDefaultRouterGatewayHelper(routerId1)
    manager.AddRouterInterfaceHelper(routerId1, subnetId1)

    netId2 := manager.CreateNetworkHelper()
    subnetId2 := manager.CreateSubnetHelper(netId2)
    routerId2 := manager.CreateRouterHelper()
    manager.SetDefaultRouterGatewayHelper(routerId2)
    manager.AddRouterInterfaceHelper(routerId2, subnetId2)

    vpcConnectionId := manager.CreateVpcConnectionHelper(routerId1, routerId2, []string{subnetId1}, []string{subnetId2})
    vpcConnection := manager.GetVpcConnection(vpcConnectionId)
    if vpcConnection.VpcConnection.FwEnabled != false ||
        !reflect.DeepEqual(vpcConnection.VpcConnection.LocalSubnets, []string{subnetId1}) ||
        !reflect.DeepEqual(vpcConnection.VpcConnection.PeerSubnets, []string{subnetId2}) {
        t.Fatal("Create vpc connection failed")
    }

    t.Cleanup(func() {
        manager.DeleteVpcConnection(vpcConnectionId)
        manager.RemoveRouterInterface(routerId1, subnetId1)
        manager.RemoveRouterInterface(routerId2, subnetId2)
        manager.DeleteRouter(routerId1)
        manager.DeleteRouter(routerId2)
        manager.DeleteNetwork(netId1)
        manager.DeleteNetwork(netId2)
    })
}

func TestCreateFirewall(t *testing.T) {
    
}
