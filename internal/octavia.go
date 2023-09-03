package internal

import (
	"encoding/json"
	"fmt"
	"github.com/valyala/fasthttp"
	"log"
	"request_openstack/configs"
	"request_openstack/consts"
	"request_openstack/internal/entity"
	"request_openstack/utils"
	"time"
)

type Octavia struct {
	Request
	headers            map[string]string
	tag                string
	snowflake          *utils.Snowflake
	ExternalNetwork    string
}

func NewLB(token string, client *fasthttp.Client) *Octavia {
	return &Octavia{
		Request: Request{
			UrlPrefix: fmt.Sprintf("http://%s:%d/v2.0/", configs.CONF.Host, consts.OctaviaPort),
			Client: client,
		},
		headers: map[string]string{"X-Auth-Token": token},
		tag: configs.CONF.Host + "_",
		snowflake: utils.NewSnowflake(uint16(1)),
		ExternalNetwork: configs.CONF.ExternalNetwork,
	}
}

// load balancer

func (l *Octavia) CreateLoadbalancer(opts entity.CreateLoadbalancerOpts) string {
	opts.Name = fmt.Sprintf("%s_%s", opts.Name, consts.LOADBALANCER)
	urlSuffix := "lbaas/loadbalancers"
	reqBody := opts.ToRequestBody()
	resp := l.Post(l.headers, urlSuffix, reqBody)
	var lb entity.LoadbalancerMap
	_ = json.Unmarshal(resp, &lb)

	//cache.RedisClient.SetMap(l.tag + consts.LOADBALANCERS, lb.Loadbalancer.Id, lb)

	l.makeSureLbActive(lb.Loadbalancer.Id)
	log.Println("==============create loadbalancer success", lb.Loadbalancer.Id)
	return lb.Loadbalancer.Id
}

func (l *Octavia) deleteLoadbalancer(ipId string) {
	urlSuffix := fmt.Sprintf("lbaas/loadbalancers/%s", ipId)
	if ok, _ := l.Delete(l.headers, urlSuffix); ok {
		log.Println("==============Delete loadbalancer success", ipId)
		return
	}
	log.Println("==============Delete loadbalancer failed", ipId)
}

func (l *Octavia) getLoadbalancer(ipId string) entity.LoadbalancerMap {
	urlSuffix := fmt.Sprintf("lbaas/loadbalancers/%s", ipId)
	resp := l.Get(l.headers, urlSuffix)
	var lb entity.LoadbalancerMap
	_ = json.Unmarshal(resp, &lb)
	return lb
}

func (l *Octavia) ListLoadbalancers() entity.Loadbalancers {
	urlSuffix := "lbaas/loadbalancers"
	resp := l.List(l.headers, urlSuffix)
	var lbs entity.Loadbalancers
	_ = json.Unmarshal(resp, &lbs)
	log.Println("==============List loadbalancers success")
	return lbs
}

func (l *Octavia) DeleteLoadbalancers() {
    lbs := l.ListLoadbalancers()
	for _, lb := range lbs.LBs {
		l.deleteLoadbalancer(lb.Id)
	}
}

// listener

func (l *Octavia) CreateListener(opts entity.CreateListenerOpts) string {
	urlSuffix := "lbaas/listeners"
	opts.Name = fmt.Sprintf("%s_%s", opts.Name, consts.LISTENER)
	reqBody := opts.ToRequestBody()
	resp := l.Post(l.headers, urlSuffix, reqBody)
	var listener entity.ListenerMap
	_ = json.Unmarshal(resp, &listener)

	//cache.RedisClient.SetMap(l.tag + consts.LISTENERS, listener.Listener.Id, listener)

	l.makeSureLbActive(opts.LoadbalancerID)
	log.Println("==============Create listener success", listener.Listener.Id)
	return listener.Listener.Id
}

func (l *Octavia) deleteListener(listener entity.Listener) {
	urlSuffix := fmt.Sprintf("lbaas/listeners/%s", listener.Id)
	if ok, _ := l.Delete(l.headers, urlSuffix); ok {
		for _, lb := range listener.Loadbalancers {
			l.makeSureLbActive(lb.Id)
		}
		log.Println("==============Delete listener success", listener.Id)
		return
	}
	log.Println("==============Delete listener failed", listener.Id)
}

func (l *Octavia) getListener(ipId string) entity.ListenerMap {
	urlSuffix := fmt.Sprintf("lbaas/listeners/%s", ipId)
	resp := l.Get(l.headers, urlSuffix)
	var listener entity.ListenerMap
	_ = json.Unmarshal(resp, &listener)
	return listener
}

func (l *Octavia) ListListeners() entity.Listeners {
	urlSuffix := "lbaas/listeners"
	resp := l.List(l.headers, urlSuffix)
	var listeners entity.Listeners
	_ = json.Unmarshal(resp, &listeners)
	log.Println("==============List listeners success")
	return listeners
}

func (l *Octavia) DeleteListeners() {
	listeners := l.ListListeners()
	for _, listener := range listeners.Liss {
		l.deleteListener(listener)
	}
}

func (l *Octavia) makeSureLbActive(lbId string) entity.LoadbalancerMap {
	lb := l.getLoadbalancer(lbId)

    for lb.Loadbalancer.ProvisioningStatus != consts.ACTIVE {
    	time.Sleep(5 * time.Second)
    	lb = l.getLoadbalancer(lbId)
	}
	//l.SyncMap(l.tag + consts.LOADBALANCERS, lbId, nil, lb)
	return lb
}

// pool

func (l *Octavia) CreatePool(opts entity.CreatePoolOpts) string {
	urlSuffix := "lbaas/pools"
	opts.Name = fmt.Sprintf("%s_%s", opts.Name, consts.POOL)
	reqBody := opts.ToRequestBody()
	resp := l.Post(l.headers, urlSuffix, reqBody)
	var pool entity.PoolMap
	_ = json.Unmarshal(resp, &pool)

	//cache.RedisClient.SetMap(l.tag + consts.POOLS, pool.Pool.Id, pool)

	l.makeSurePoolActive(pool.Pool.Id)
	for _, lb := range pool.Pool.Loadbalancers {
		l.makeSureLbActive(lb.Id)
	}
	log.Println("==============create pool success", pool.Pool.Id)
	return pool.Pool.Id
}

func (l *Octavia) deletePool(pool entity.Pool) {
	urlSuffix := fmt.Sprintf( "lbaas/pools/%s", pool.Id)
	if ok, _ := l.Delete(l.headers, urlSuffix); ok {
		for _, lb := range pool.Loadbalancers {
			l.makeSureLbActive(lb.Id)
		}
		log.Println("==============Delete pool success", pool.Id)
		return
	}
	log.Println("==============delete pool failed", pool.Id)
}

func (l *Octavia) getPool(ipId string) entity.PoolMap {
	urlSuffix := fmt.Sprintf("lbaas/pools/%s", ipId)
	resp := l.Get(l.headers, urlSuffix)
	var pool entity.PoolMap
	_ = json.Unmarshal(resp, &pool)
	return pool
}

func (l *Octavia) ListPools() entity.Pools {
	urlSuffix := "lbaas/pools"
	resp := l.List(l.headers, urlSuffix)
	var pools entity.Pools
	_ = json.Unmarshal(resp, &pools)
	log.Println("==============List pools success")
	return pools
}

func (l *Octavia) DeletePools() {
	pools := l.ListPools()
	for _, pool := range pools.Ps {
		for _, member := range pool.Members {
			l.deletePoolMember(pool, member.(map[string]interface{})["id"].(string))
		}
		l.deletePool(pool)
	}
}

func (l *Octavia) makeSurePoolActive(poolId string) entity.PoolMap {
	pool := l.getPool(poolId)

	for pool.Pool.ProvisioningStatus != consts.ACTIVE {
		time.Sleep(5 * time.Second)
		pool = l.getPool(poolId)
	}
	//l.SyncMap(l.tag + consts.POOLS, poolId, nil, pool)
	return pool
}

// pool member

func (l *Octavia) CreatePoolMember(poolId string, opts entity.CreateMemberOpts) string {
	urlSuffix := fmt.Sprintf("lbaas/pools/%s/members", poolId)
	opts.Name = fmt.Sprintf("%s_%s", opts.Name, consts.POOL)
	reqBody := opts.ToRequestBody()
	resp := l.Post(l.headers, urlSuffix, reqBody)
	var member entity.MemberMap
	_ = json.Unmarshal(resp, &member)

	//cache.RedisClient.SetMap(l.tag + consts.MEMBERS, member.Member.Id, member)
	log.Println("==============Create member success", member.Member.Id)

	l.makeSurePoolActive(poolId)
	return member.Member.Id
}

func (l *Octavia) deletePoolMember(pool entity.Pool, memberId string) {
	urlSuffix := fmt.Sprintf("lbaas/pools/%s/members/%s", pool.Id, memberId)
	if ok, _ := l.Delete(l.headers, urlSuffix); ok {
		l.makeSurePoolActive(pool.Id)
		for _, lb := range pool.Loadbalancers {
			l.makeSureLbActive(lb.Id)
		}
		log.Println("==============Delete member success", memberId)
		return
	}
	log.Println("==============Delete member failed", memberId)
}

func (l *Octavia) getPoolMember(poolId, memberId string) entity.MemberMap {
	urlSuffix := fmt.Sprintf("lbaas/pools/%s/members/%s", poolId, memberId)
	resp := l.Get(l.headers, urlSuffix)
	var member entity.MemberMap
	_ = json.Unmarshal(resp, &member)
	return member
}

func (l *Octavia) ListPoolMembers(poolId string) entity.Members {
	urlSuffix := fmt.Sprintf("lbaas/pools/%s/members", poolId)
	resp := l.List(l.headers, urlSuffix)
	var ms entity.Members
	_ = json.Unmarshal(resp, &ms)
	log.Println("==============List pool members success")
	return ms
}

// health monitor

func (l *Octavia) CreateHealthMonitor(opts entity.CreateHealthMonitorOpts) string {
	urlSuffix := "lbaas/healthmonitors"
	opts.Name = fmt.Sprintf("%s_%s", opts.Name, consts.HEALTHMONITOR)
	reqBody := opts.ToRequestBody()
	resp := l.Post(l.headers, urlSuffix, reqBody)
	var healthMonitor entity.HealthMonitorMap
	_ = json.Unmarshal(resp, &healthMonitor)

	//cache.RedisClient.SetMap(l.tag + consts.HEALTHMONITORS, healthmonitor.Healthmonitor.Id, healthmonitor)
	log.Println("==============Create health monitor success", healthMonitor.Healthmonitor.Id)
	return healthMonitor.Healthmonitor.Id
}

func (l *Octavia) deleteHealthMonitor(healthmonitorId string) {
	urlSuffix := fmt.Sprintf("lbaas/healthmonitors/%s", healthmonitorId)
	if ok, _ := l.Delete(l.headers, urlSuffix); ok {
		log.Println("==============Delete healthmonitor success", healthmonitorId)
		return
	}
	log.Println("==============Delete healthmonitor failed", healthmonitorId)
}

func (l *Octavia) getHealthMonitor(healthmonitorId string) entity.HealthMonitorMap {
	urlSuffix := fmt.Sprintf("lbaas/healthmonitors/%s", healthmonitorId)
	resp := l.Get(l.headers, urlSuffix)
	var healthmonitor entity.HealthMonitorMap
	_ = json.Unmarshal(resp, &healthmonitor)
	return healthmonitor
}

func (l *Octavia) ListHealthMonitors() entity.HealthMonitors {
	urlSuffix := "lbaas/healthmonitors"
	resp := l.List(l.headers, urlSuffix)
	var hms entity.HealthMonitors
	_ = json.Unmarshal(resp, &hms)
	log.Println("==============List health monitors success")
	return hms
}

func (l *Octavia) DeleteHealthMonitors() {
	healthmonitors := l.ListHealthMonitors()
	for _, healthmonitor := range healthmonitors.HMs {
		l.deleteHealthMonitor(healthmonitor.Id)
	}
}

// L7 policy

func (l *Octavia) CreateL7Policy(listenerId string) string {
	urlSuffix := "lbaas/l7policies"
	reqBody := fmt.Sprintf("{\"l7policy\": {\"description\": \"Redirect requests to example.com\", \"admin_state_up\": true, \"listener_id\": \"%+v\", \"redirect_url\": \"http://www.example.com\", \"redirect_http_code\": 301, \"name\": \"redirect-example.com\", \"action\": \"REDIRECT_TO_URL\", \"position\": 1, \"tags\": [\"test_tag\"]}}", listenerId)
	resp := l.Post(l.headers, urlSuffix, reqBody)
	var l7policy entity.L7PolicyMap
	_ = json.Unmarshal(resp, &l7policy)

	//cache.RedisClient.SetMap(l.tag + consts.L7POLICIES, l7policy.L7Policy.Id, l7policy)
	l.makeSureL7PolicyActive(l7policy.L7Policy.Id)
	log.Println("==============create l7policy success", l7policy.L7Policy.Id)
	return l7policy.L7Policy.Id
}

func (l *Octavia) deleteL7Policy(l7policyId string) {
	urlSuffix := fmt.Sprintf("lbaas/l7policies/%s", l7policyId)
	if ok, _ := l.Delete(l.headers, urlSuffix); ok {
		log.Println("==============Delete l7policy success", l7policyId)
		return
	}
	log.Println("==============Delete l7policy failed", l7policyId)
}

func (l *Octavia) getL7Policy(l7policyId string) entity.L7PolicyMap {
	urlSuffix := fmt.Sprintf("lbaas/l7policies/%s", l7policyId)
	resp := l.Get(l.headers, urlSuffix)
	var l7policy entity.L7PolicyMap
	_ = json.Unmarshal(resp, &l7policy)
	return l7policy
}

func (l *Octavia) ListL7Policies() entity.L7Policies {
	urlSuffix := "lbaas/l7policies"
	resp := l.List(l.headers, urlSuffix)
	var l7ps entity.L7Policies
	_ = json.Unmarshal(resp, &l7ps)
	log.Println("==============List l7 policies success")
	return l7ps
}

func (l *Octavia) makeSureL7PolicyActive(l7PolicyId string) entity.L7PolicyMap {
	l7Policy := l.getL7Policy(l7PolicyId)

	for l7Policy.L7Policy.ProvisioningStatus != consts.ACTIVE {
		time.Sleep(5 * time.Second)
		l7Policy = l.getL7Policy(l7PolicyId)
	}
	//l.SyncMap(l.tag + consts.L7POLICIES, l7PolicyId, nil, l7Policy)
	return l7Policy
}

func (l *Octavia) DeleteL7Policies() {
	l7policies := l.ListL7Policies()
	for _, l7policy := range l7policies.L7Ps {
		for _, rule := range l7policy.Rules {
			l.deleteL7Rule(l7policy.Id, rule.Id)
		}
		l.deleteL7Policy(l7policy.Id)
	}
}

// L7 rule

func (l *Octavia) CreateL7Rule(policyId string) string {
	urlSuffix := fmt.Sprintf("lbaas/l7policies/%s/rules", policyId)
	reqBody := fmt.Sprintf("{\"rule\": {\"compare_type\": \"REGEX\", \"invert\": false, \"type\": \"PATH\", \"value\": \"/images*\", \"admin_state_up\": true, \"tags\": [\"test_tag\"]}}")
	resp := l.Post(l.headers, urlSuffix, reqBody)
	var l7Rule entity.L7RuleMap
	_ = json.Unmarshal(resp, &l7Rule)

	//cache.RedisClient.SetMap(l.tag + consts.L7RULES, l7Rule.Rule.Id, l7Rule)
	log.Println("==============create l7Rule success", l7Rule.Rule.Id)
	return l7Rule.Rule.Id
}

func (l *Octavia) deleteL7Rule(l7PolicyId, l7RuleId string) {
	urlSuffix := fmt.Sprintf("lbaas/l7policies/%s/rules/%s", l7PolicyId, l7RuleId)
	if ok, _ := l.Delete(l.headers, urlSuffix); ok {
		l.makeSureL7PolicyActive(l7PolicyId)
		log.Println("==============Delete l7Rule success", l7RuleId)
		return
	}
	log.Println("==============Delete l7Rule failed", l7RuleId)
}

func (l *Octavia) getL7Rule(l7PolicyId, l7RuleId string) entity.L7RuleMap {
	urlSuffix := fmt.Sprintf("lbaas/l7policies/%s/rules/%s", l7PolicyId, l7RuleId)
	resp := l.Get(l.headers, urlSuffix)
	var l7Rule entity.L7RuleMap
	_ = json.Unmarshal(resp, &l7Rule)
	return l7Rule
}
