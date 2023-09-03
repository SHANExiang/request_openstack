package entity

import (
	"fmt"
	"reflect"
	"request_openstack/configs"
	"request_openstack/consts"
	"strings"
	"time"
)

type Floatingip struct {
	FixedIpAddress    string    `json:"fixed_ip_address"`
	FloatingIpAddress string    `json:"floating_ip_address"`
	FloatingNetworkId string    `json:"floating_network_id"`
	Id                string    `json:"id"`
	PortId            string    `json:"port_id"`
	RouterId          string    `json:"router_id"`
	Status            string    `json:"status"`
	ProjectId         string    `json:"project_id"`
	TenantId          string    `json:"tenant_id"`
	Description       string    `json:"description"`
	DnsDomain         string    `json:"dns_domain"`
	DnsName           string    `json:"dns_name"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	RevisionNumber    int       `json:"revision_number"`
	PortDetails       struct {
		Status       string `json:"status"`
		Name         string `json:"name"`
		AdminStateUp bool   `json:"admin_state_up"`
		NetworkId    string `json:"network_id"`
		DeviceOwner  string `json:"device_owner"`
		MacAddress   string `json:"mac_address"`
		DeviceId     string `json:"device_id"`
	} `json:"port_details"`
	Tags            []string      `json:"tags"`
	PortForwardings []interface{} `json:"port_forwardings"`
	QosPolicyId     string        `json:"qos_policy_id"`
}

type FipMap struct {
	Floatingip `json:"floatingip"`
}

type Fips struct {
	Fs                 []Floatingip `json:"floatingips"`
	Count              int   `json:"count"`
}

type CreateFipOpts struct {
	Description       string `json:"description,omitempty"`
	FloatingNetworkID string `json:"floating_network_id" required:"true"`
	FloatingIP        string `json:"floating_ip_address,omitempty"`
	PortID            string `json:"port_id,omitempty"`
	FixedIP           string `json:"fixed_ip_address,omitempty"`
	SubnetID          string `json:"subnet_id,omitempty"`
	TenantID          string `json:"tenant_id,omitempty"`
	ProjectID         string `json:"project_id,omitempty"`
}

func (opts *CreateFipOpts) ToRequestBody() string {
	reqBody, err := BuildRequestBody(opts, consts.FLOATINGIP)
	if err != nil {
		panic(fmt.Sprintf("Failed to build request body %s", err))
	}
	return reqBody
}

type UpdateFipOpts struct {
	Description *string `json:"description,omitempty"`
	PortID      *string `json:"port_id,omitempty"`
	FixedIP     string  `json:"fixed_ip_address,omitempty"`
}

func (opts *UpdateFipOpts) ToRequestBody() string {
	reqBody, err := BuildRequestBody(opts, consts.FLOATINGIP)
	if err != nil {
		panic(fmt.Sprintf("Failed to build request body %s", err))
	}
	return reqBody
}

func (opts *CreateFipOpts) AssignProps(props map[string]interface{}) (*CreateFipOpts, map[string]string) {
	var deps = make(map[string]string)
	typ := reflect.TypeOf(opts)
	val := reflect.ValueOf(opts).Elem()
	for i := 0;i < typ.Elem().NumField();i++ {
		field := typ.Elem().Field(i)
		tag := field.Tag.Get("json")
		compareFieldName := strings.Split(tag, ",")[0]
		value := val.Field(i)
		fieldType := field.Type
		if v, ok := props[compareFieldName]; ok {
			switch fieldType.Kind() {
			case reflect.String:
				if compareFieldName == "floating_network_id" && v.(string) == "local"{
					value.SetString(configs.CONF.ExternalNetwork)
				} else if compareFieldName == "port_id" && !IsUUID(v.(string)) {
					deps[v.(string)] = field.Name
				} else {
					value.SetString(v.(string))
				}
			}
		}
	}
	return opts, deps
}
