package internal

import (
	"encoding/json"
	"fmt"
	"github.com/valyala/fasthttp"
	clientv3 "go.etcd.io/etcd/client/v3"
	"log"
	"request_openstack/configs"
	"request_openstack/consts"
	"request_openstack/internal/cache"
	"request_openstack/internal/entity"
	"request_openstack/internal/etcd"
)

type Keystone struct {
	Headers               map[string]string
	Request
	tag                   string
}

func NewKeystone(client  *fasthttp.Client) *Keystone {
	return &Keystone{
		Request: Request{
		     UrlPrefix: fmt.Sprintf("http://%s:%d/v3", configs.CONF.Host, consts.KeystonePort),
		     Client: client,
	    },
		Headers: make(map[string]string),
		tag: configs.CONF.Host + "_",
	}
}

func (k *Keystone) GetToken(projectName, userName, userPassword string) string {
	urlSuffix := "/auth/tokens"
	reqBody := k.constructAuthReqBody(projectName, userName, userPassword)
	token := k.GetHeaderToken(k.Headers, urlSuffix, reqBody)
	log.Println("==============Get token success")
	return token
}

func (k *Keystone) GetFederationToken(projectName, userName, userPassword string) string {
	urlSuffix := "/auth/tokens"
	reqBody := k.constructAuthReqBody(projectName, userName, userPassword)
	token := k.GetHeaderToken(k.Headers, urlSuffix, reqBody)
	log.Println("==============Get token success")
	return token
}



func (k *Keystone) setToken(projectName, userName, userPassword string) string {
	log.Println("Etcd server no token, get from openstack then put to etcd server")
	token := k.GetToken(projectName, userName, userPassword)
	g := etcd.GrantLease(2 * 60 * 60)
	etcd.PutKV(configs.CONF.Host + "_" + projectName + "_token", token, clientv3.WithLease(g.ID))
	return token
}

func (k *Keystone) AllocateToken(projectName, userName, userPassword string) (token string) {
	tokenKey := configs.CONF.Host + "_" + projectName + "_token"
	v := etcd.GetKV(tokenKey)
	if v == "" {
		token = k.setToken(projectName, userName, userPassword)
	} else {
		token = v
	}
	k.Headers["X-Auth-Token"] = token
	return
}

func (k *Keystone) constructAuthReqBody(projectName, userName, userPassword string) string {
	bodyStr := fmt.Sprintf("{\"auth\": {\"identity\": {\"methods\": [\"password\"],\"password\": {\"user\": {\"name\": \"%+v\",\"domain\": {\"id\": \"default\"},\"password\": \"%+v\"}}},\"scope\": {\"project\": {\"domain\": {\"name\": \"default\"},\"name\": \"%+v\"}}}}", userName, userPassword, projectName)
	return bodyStr
}

func (k *Keystone) GetAdminProjectId(token string) string {
	key := fmt.Sprintf("%s_admin", configs.CONF.Host)
	v := etcd.GetKV(key)
	if v == "" {
		urlSuffix := "/projects?name=admin"
		k.Headers["X-Auth-Token"] = token
		res := k.DecorateGetResp(k.List)(k.Headers, urlSuffix)
		projectId := parseProjectId(res)
		etcd.PutKV(key, projectId)
		return projectId
	} else {
		return v
	}
}



func (k *Keystone) GetProjectId(projectName string) string {
	urlSuffix := fmt.Sprintf("/projects?name=%s", projectName)
	res := k.DecorateGetResp(k.List)(k.Headers, urlSuffix)
	projectId := parseProjectId(res)
	log.Println("==============Get project success")
	return projectId
}

func (k *Keystone) createProject(projectName string) string {
	urlSuffix := "/projects"
	reqBody := fmt.Sprintf("{\"project\": {\"description\": \"My new project\", \"domain_id\": \"default\", \"enabled\": true, \"is_domain\": false, \"name\": \"%+v\", \"options\": {}}}", projectName)
    resp := k.Post(k.Headers, urlSuffix, reqBody)

    var project entity.ProjectMap
    err := json.Unmarshal(resp, &project)
    if err != nil {
    	log.Println("json unmarshal failed", err)
    	panic("")
	}
    //cache.RedisClient.SetMap(k.tag + consts.PROJECTS, project.Project.Id, project)
	log.Println("==============Create project success", project.Project.Id)
    return project.Project.Id
}

func (k *Keystone) DeleteProject(projectId string) {
	urlSuffix := fmt.Sprintf("/projects/%s", projectId)
	resp, _ := k.Delete(k.Headers, urlSuffix)
	if resp {
		//cache.RedisClient.DeleteMap(k.tag + consts.PROJECTS, projectId)
		log.Println("==============Delete project success", projectId)
	} else {
		log.Println("==============Delete project failed", projectId)
	}
}

func (k *Keystone) createUser(projectId, userName, userPassword string) string {
	urlSuffix := "/users"
	reqBody := fmt.Sprintf("{\"user\": {\"default_project_id\": \"%+v\", \"domain_id\": \"default\", \"enabled\": true, \"name\": \"%+v\", \"password\": \"%+v\", \"description\": \"sdn test user\", \"options\": {\"ignore_password_expiry\": true}}}", projectId, userName, userPassword)
	resp := k.Post(k.Headers, urlSuffix, reqBody)

	var user entity.UserMap
	err := json.Unmarshal(resp, &user)
	if err != nil {
		log.Println("json unmarshal failed", err)
		panic("")
	}

	//cache.RedisClient.SetMap(k.tag + consts.USERS, user.User.Id, user)
	log.Println("==============Create user success", userName)
	return user.User.Id
}

func (k *Keystone) SetUserPasswordNotExpire(userId string) string {
	k.Headers["Content-Type"] = consts.ContentTypeJson
	urlSuffix := fmt.Sprintf("/users/%s", userId)
	reqBody := fmt.Sprintf("{\"user\": {\"options\": {\"ignore_password_expiry\": true}}}")
	resp := k.Patch(k.Headers, urlSuffix, reqBody)

	var user entity.UserMap
	err := json.Unmarshal(resp, &user)
	if err != nil {
		log.Println("json unmarshal failed", err)
		panic("")
	}

	log.Println("==============Update user success", userId)
	return user.User.Id
}

func (k *Keystone) getUser(userId string) entity.UserMap {
	urlSuffix := fmt.Sprintf("/users/%s", userId)
	resp := k.Get(k.Headers, urlSuffix)

	var user entity.UserMap
	if err := json.Unmarshal(resp, &user); err != nil {
		log.Println("json unmarshal failed", err)
		panic("")
	}
	return user
}

func (k *Keystone) GetUserByName(userName string) string {
	urlSuffix := fmt.Sprintf("/users?name=%s", userName)
	res := k.DecorateGetResp(k.List)(k.Headers, urlSuffix)
	userId := parseUserId(res)
	log.Println("==============List user success")
	return userId
}

func (k *Keystone) DeleteUserByName(userName string) {
	userId := k.GetUserByName(userName)
	k.DeleteUser(userId)
}

func (k *Keystone) DeleteProjectByName(projectName string) {
	projectId := k.GetProjectId(projectName)
	k.DeleteProject(projectId)
}

func (k *Keystone) DeleteUser(userId string) {
	urlSuffix := fmt.Sprintf("/users/%s", userId)
	resp, _ := k.Delete(k.Headers, urlSuffix)
    if resp {
    	//cache.RedisClient.DeleteMap(k.tag + consts.USERS, userId)
		log.Println("==============Delete user success", userId)
	} else {
		log.Println("==============Delete user failed", userId)
	}
}

func (k *Keystone) getAdminRole() string {
	urlSuffix := "/roles?name=admin"
	resp := k.DecorateGetResp(k.List)(k.Headers, urlSuffix)
	roles := resp["roles"].([]interface{})
	adminRole := roles[0].(map[string]interface{})
	return adminRole["id"].(string)
}

func (k *Keystone) assignRoleToUser(projectId, userId string) {
    adminRoleId := k.getAdminRole()
    urlSuffix := fmt.Sprintf("/projects/%s/users/%s/roles/%s", projectId, userId, adminRoleId)
    k.Put(k.Headers, urlSuffix, "")

	//k.SyncMap(k.tag + consts.USERS, userId, func(resourceId string) interface{} {
	//	return k.getUser(userId)
	//}, nil)
}

func (k *Keystone) PrepareProjectUserToken(projectName, userName, userPassword string) (string, string) {
	projectId := k.GetProjectId(projectName)
	if projectId == "" {
		projectId = k.createProject(projectName)
	}
	userId := k.GetUserByName(userName)
	if userId == "" {
		userId = k.createUser(projectId, userName, userPassword)
		k.assignRoleToUser(projectId, userId)
	}
    token := k.AllocateToken(projectName, userName, userPassword)
    return projectId, token
}

func (k *Keystone) DeleteProjectUser() {
	userIds := cache.RedisClient.GetMaps(k.tag + consts.USERS)
	for _, userId := range userIds {
		k.DeleteUser(userId)
	}

	projectIds := cache.RedisClient.GetMaps(k.tag + consts.PROJECTS)
	for _, projectId := range projectIds {
		k.DeleteProject(projectId)
	}
}

func (k *Keystone) MakeSureProjectExist() string {
	projectName := configs.CONF.ProjectName
	userName := configs.CONF.UserName
	userPassword := configs.CONF.UserPassword
	projectId := k.GetProjectId(projectName)
	if projectId == "" {
		projectId = k.createProject(projectName)
	}
	userId := k.GetUserByName(userName)
	if userId == "" {
		userId = k.createUser(projectId, userName, userPassword)
		k.assignRoleToUser(projectId, userId)
	}
	return projectId
}

func (k *Keystone) SetHeader(key, val string)  {
	k.Headers[key] = val
}

type ProjectResp struct {
	Links struct {
		Self     string      `json:"self"`
		Previous interface{} `json:"previous"`
		Next     interface{} `json:"next"`
	} `json:"links"`
	Projects []struct {
		IsDomain    bool   `json:"is_domain"`
		Description string `json:"description"`
		Links       struct {
			Self string `json:"self"`
		} `json:"links"`
		Tags     []interface{} `json:"tags"`
		Enabled  bool          `json:"enabled"`
		Id       string        `json:"id"`
		ParentId string        `json:"parent_id"`
		Options  struct {
		} `json:"options"`
		DomainId string `json:"domain_id"`
		Name     string `json:"name"`
	} `json:"projects"`
}

func parseProjectId(resp map[string]interface{}) string {
    if v, exist := resp["projects"]; exist {
    	for _, project := range v.([]interface{}) {
    		return project.(map[string]interface{})["id"].(string)
		}
	}
	log.Println("Get project_id None")
	return ""
}

func parseUserId(resp map[string]interface{}) string {
	if v, exist := resp["users"]; exist {
		for _, project := range v.([]interface{}) {
			return project.(map[string]interface{})["id"].(string)
		}
	}
	log.Println("Get user id None")
	return ""
}
