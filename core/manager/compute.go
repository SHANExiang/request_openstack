package manager

import (
	"log"
	"request_openstack/configs"
	"request_openstack/internal/entity"
)

func (m *Manager) CreateAndSetAggregate(flavorKey, flavorVal string) {
	createAggregateOpts := entity.CreateAggregateOpts{Name: flavorKey}
	aggregateId := m.CreateAggregate(createAggregateOpts)

	hosts := m.GetComputeHosts()
	for _, host := range hosts {
		addHostOpts := entity.AddHostOpts{
			Host: host,
		}
		m.AggregateAddHost(aggregateId, addHostOpts)
	}
	metadataOpts := entity.SetMetadataOpts{Metadata: map[string]interface{}{flavorKey: flavorVal}}
    m.AggregateSetMetadata(aggregateId, metadataOpts)
}

func (m *Manager) CreateInstanceForTest() {
	name := "dx_test1"
	netId, _, routerId := m.CreateVpc()
	updateRouterOpts := &entity.UpdateRouterOpts{GatewayInfo: &entity.GatewayInfo{NetworkID: configs.CONF.ExternalNetwork}}
	m.UpdateRouter(routerId, updateRouterOpts)
	m.EnsureSgExist(configs.CONF.UserName)
	instanceOpts := entity.CreateInstanceOpts{
		FlavorRef:      configs.CONF.FlavorId,
		ImageRef:       configs.CONF.ImageId,
		Networks:       []entity.ServerNet{{UUID: netId}},
		AdminPass:      "Wang.123",
		SecurityGroups: []entity.ServerSg{{Name: configs.CONF.ProjectName}},
		Name:           name,
		BlockDeviceMappingV2: []entity.BlockDeviceMapping{{
			BootIndex: 0, Uuid: configs.CONF.ImageId, SourceType: "image",
			DestinationType: "volume", VolumeSize: 10, DeleteOnTermination: true,
		}},
	}
	m.CreateInstance(&instanceOpts)
	m.CreateInstance(&instanceOpts)
}

func (m *Manager) GetInstancesSysDisk(instanceId string) string {
	instance := m.GetInstanceDetail(instanceId)
	val := instance.Server.OsExtendedVolumesVolumesAttached
	for _, v := range val {
		volumeId := v.(map[string]interface{})["id"].(string)
		if m.CheckSysOrDataDisk(volumeId) {
			return volumeId
		}
	}
	log.Println("==============Not found sys disk", instanceId)
	return ""
}