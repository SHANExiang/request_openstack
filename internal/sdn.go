package internal

import (
	"encoding/json"
	"fmt"
	"log"
	"request_openstack/configs"
	"request_openstack/consts"
	"request_openstack/internal/entity"
)

type SDN struct {
	Request
	headers            map[string]string
}

func NewSDN() *SDN {
	sdnObj := &SDN{
		Request: Request{
			UrlPrefix: fmt.Sprintf("https://%s:%d/", configs.CONF.SDN.SDNHost, consts.SDNPort),
			Client: NewSSLClient(),
		},
		headers: make(map[string]string),
	}
	token := sdnObj.GetSDNToken()
	sdnObj.SetToken(token)
	return sdnObj
}

func (s *SDN) GetSDNToken() string {
	urlSuffix := "controller/v2/tokens"
    opts := entity.CreateSDNTokenOpts{
    	UserName: configs.CONF.SDNUserName,
    	Password: configs.CONF.SDNPassword,
	}
	reqBody := opts.ToRequestBody()
    resp := s.Post(s.headers, urlSuffix, reqBody)
    var sdnToken entity.SDNToken
    _ = json.Unmarshal(resp, &sdnToken)
    log.Println("==============Get SDN token success", sdnToken.Data.TokenId)
    return sdnToken.Data.TokenId
}

func (s *SDN) SetToken(token string) {
	s.headers["X-ACCESS-TOKEN"] = token
}

func (s *SDN) ListSDNNetworks() {
	urlSuffix := "restconf/data/huawei-ac-neutron:neutron-cfg/networks"
	resp := s.List(s.headers, urlSuffix)
	var networks entity.SDNNetworks
	_ = json.Unmarshal(resp, &networks)
	log.Println(networks)
	log.Println("==============List SDN networks success, there had", len(networks.HuaweiAcNeutronNetworks.Network))
}

func (s *SDN) GetSDNLogicNetworks() {
	urlSuffix := "controller/dc/v3/logicnetwork/networks"
	resp := s.List(s.headers, urlSuffix)
	log.Println("==============List SDN networks success", string(resp))
}

func (s *SDN) GetSDNLogicRouters() {
	urlSuffix := "controller/dc/v3/logicnetwork/routers"
	resp := s.List(s.headers, urlSuffix)
	log.Println("==============List SDN logic routers success", string(resp))
}

func (s *SDN) CreateSDNVpcConnect() {
	urlSuffix := "controller/dc/v3/servicenetwork/vpc-connects"
	formatter := `{
                      "vpc-connect": [{
                          "id": "a915f8c1-d06f-46cd-a968-38328e0ab2d0",
                          "name": "vpc001",
                          "description": "This is vpc",
			              "localLogicRouterId": "d0733770-bb85-4ea6-abda-314622dd8ae0",
			              "localCidrs": ["192.5.5.0/24"],
			              "peerLogicRouterId": "67833ffe-d65e-4e99-a337-c4b281c178ec",
			              "peerCidrs": ["192.47.47.0/24"],
                          "tenantId": "f4e08f96-a401-45bc-aa5b-8a93bcdb9813",
                          "peerTenantId": "f4e08f96-a401-45bc-aa5b-8a93bcdb9813"
                      }]
                   }`
	resp := s.Post(s.headers, urlSuffix, formatter)
	log.Println(string(resp))
	log.Println("==============Create SDN Vpc connect success")
}

func (s *SDN) DeleteSDNVpcConnect(vpc_connect_id string) {
	urlSuffix := fmt.Sprintf("controller/dc/v3/servicenetwork/vpc-connects/vpc-connect/%s", vpc_connect_id)
	if ok, _ := s.Delete(s.headers, urlSuffix); ok {
		log.Println("==============Delete SDN Vpc connect success")
		return
	}
	log.Println("==============Delete SDN Vpc connect failed")
}

func (s *SDN) GetMDCVpcConnects() {
	urlSuffix := "controller/mdc/v3/vpc-connects"
	resp := s.List(s.headers, urlSuffix)
	log.Println(string(resp))
}

func (s *SDN) ListSnats() {
	urlSuffix := "/controller/dc/v2/neutronapi/snat"
	resp := s.List(s.headers, urlSuffix)
	log.Println(string(resp))
}

func (s *SDN) DeleteRouterGateway() {
	routerId := "3273a04f-08e9-47b7-90d3-d8ba113d3569"
	urlSuffix := fmt.Sprintf("/restconf/data/huawei-ac-neutron:neutron-cfg/routers/router/%s", routerId)
	reqBody := `{
	"router" : [
		{
			"uuid" : "3273a04f-08e9-47b7-90d3-d8ba113d3569",
            "gateway-port-id": null,
            "external-gateway-info": {}
}]}
`
	resp := s.Put(s.headers, urlSuffix, reqBody)
	log.Println(string(resp))
}
