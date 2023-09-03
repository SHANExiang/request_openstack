package entity

import "time"

type Port struct {
	AllowedAddressPairs []interface{} `json:"allowed_address_pairs"`
	ExtraDhcpOpts       []interface{} `json:"extra_dhcp_opts"`
	UpdatedAt           time.Time     `json:"updated_at"`
	DeviceOwner         string        `json:"device_owner"`
	RevisionNumber      int           `json:"revision_number"`
	PortSecurityEnabled bool          `json:"port_security_enabled"`
	BindingProfile      struct {
	} `json:"binding:profile"`
	FixedIps []struct {
		SubnetId  string `json:"subnet_id"`
		IpAddress string `json:"ip_address"`
	} `json:"fixed_ips"`
	Id                string        `json:"id"`
	SecurityGroups    []interface{} `json:"security_groups"`
	BindingVifDetails struct {
	} `json:"binding:vif_details"`
	BindingVifType  string        `json:"binding:vif_type"`
	MacAddress      string        `json:"mac_address"`
	ProjectId       string        `json:"project_id"`
	Status          string        `json:"status"`
	BindingHostId   string        `json:"binding:host_id"`
	Description     string        `json:"description"`
	Tags            []interface{} `json:"tags"`
	QosPolicyId     interface{}   `json:"qos_policy_id"`
	Name            string        `json:"name"`
	AdminStateUp    bool          `json:"admin_state_up"`
	NetworkId       string        `json:"network_id"`
	TenantId        string        `json:"tenant_id"`
	CreatedAt       time.Time     `json:"created_at"`
	BindingVnicType string        `json:"binding:vnic_type"`
	DeviceId        string        `json:"device_id"`
}

type PortMap struct {
	Port              `json:"port"`
}

type Ports struct {
	Count    int    `json:"count"`
	Ps       []Port `json:"ports"`
}
