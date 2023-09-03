package entity

import "fmt"

type CreateSDNTokenOpts struct {
	UserName            string        	`json:"userName"`
	Password            string          `json:"password"`
}

type SDNToken struct {
	Data struct {
		TokenId     string `json:"token_id"`
		ExpiredDate string `json:"expiredDate"`
	} `json:"data"`
	Errcode string `json:"errcode"`
	Errmsg  string `json:"errmsg"`
}

func (opts *CreateSDNTokenOpts) ToRequestBody() string {
	reqBody, err := BuildRequestBody(opts, "")
	if err != nil {
		panic(fmt.Sprintf("Failed to build request body %s", err))
	}
	return reqBody
}

type SDNNetworks struct {
	HuaweiAcNeutronNetworks struct {
		Network []struct {
			Uuid                                      string `json:"uuid"`
			Mtu                                       int    `json:"mtu"`
			VlanTransparent                           bool   `json:"vlan-transparent"`
			Name                                      string `json:"name"`
			Description                               string `json:"description"`
			UpdatedAt                                 string `json:"updated-at"`
			CreatedAt                                 string `json:"created-at"`
			TenantName                                string `json:"tenant-name"`
			AdminStateUp                              bool   `json:"admin-state-up"`
			CloudName                                 string `json:"cloud-name"`
			Shared                                    bool   `json:"shared"`
			TenantId                                  string `json:"tenant-id"`
			HuaweiAcNeutronProviderExtPhysicalNetwork string `json:"huawei-ac-neutron-provider-ext:physical-network"`
			HuaweiAcNeutronProviderExtNetworkType     string `json:"huawei-ac-neutron-provider-ext:network-type"`
			HuaweiAcNeutronProviderExtSegmentationId  string `json:"huawei-ac-neutron-provider-ext:segmentation-id"`
			HuaweiAcNeutronL3ExtExternal              bool   `json:"huawei-ac-neutron-l3-ext:external"`
		} `json:"network"`
	} `json:"huawei-ac-neutron:networks"`
}
