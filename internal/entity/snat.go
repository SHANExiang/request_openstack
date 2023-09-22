package entity

import (
    "fmt"
    "request_openstack/consts"
)

type Snat struct {
    Id            string `json:"id,omitempty"`
    Name          string `json:"name,omitempty"`
    TenantId      string `json:"tenant_id" required:"true"`
    RouterId      string `json:"router_id" required:"true"`
    SnatNetworkId string `json:"snat_network_id" required:"true"`
    SnatIpAddress string `json:"snat_ip_address"`
    SnatIpPool    []struct {
        BeginIp string `json:"begin_ip"`
        EndIp   string `json:"end_ip"`
    } `json:"snat_ip_pool,omitempty"`
    OriginalCidrs []string `json:"original_cidrs" required:"true"`
}

type SnatMap struct {
     Snat        `json:"snat"`
}

func (opts *Snat) ToRequestBody() string {
    reqBody, err := BuildRequestBody(opts, consts.Snat)
    if err != nil {
        panic(fmt.Sprintf("Failed to build request body %s", err))
    }
    return reqBody
}

type Snats struct {
    Ss           []Snat         `json:"snats"`
}
