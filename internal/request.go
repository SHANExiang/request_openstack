package internal

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"github.com/valyala/fasthttp"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"request_openstack/consts"
	"request_openstack/internal/cache"
	"time"
)

type Request struct {
	UrlPrefix        string
	Client           *fasthttp.Client
}

func NewClient() (client *fasthttp.Client) {
	readTimeout, _ := time.ParseDuration("5000000ms")
	writeTimeout, _ := time.ParseDuration("5000000ms")
	maxIdleConnDuration, _ := time.ParseDuration("1h")
	client = &fasthttp.Client{
		ReadTimeout:                   readTimeout,
		WriteTimeout:                  writeTimeout,
		MaxIdleConnDuration:           maxIdleConnDuration,
		NoDefaultUserAgentHeader:      true, // Don't send: User-Agent: fasthttp
		DisableHeaderNamesNormalizing: true, // If you set the case on your headers correctly you can enable this
		DisablePathNormalizing:        true,
		// increase DNS cache time to an hour instead of default minute
		Dial: (&fasthttp.TCPDialer{
			Concurrency:      4096,
			DNSCacheDuration: time.Hour,
		}).Dial,
	}
	return client
}

func loadCACert() *x509.CertPool {
	exePath, err := os.Getwd()
	if err != nil {
		fmt.Printf("获取可执行文件路径失败：%s\n", err)
		return nil
	}
	//certPath := filepath.Join(exePath, "configs", "ssl_cacert.pem")
	certPath := filepath.Join(exePath, "configs", "Huawei Equipment Root CA.pem")
	caCert, err := ioutil.ReadFile(certPath) // CA 证书文件路径
	if err != nil {
		panic(err)
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	return caCertPool

}

func NewSSLClient() (client *fasthttp.Client) {
	readTimeout, _ := time.ParseDuration("500000ms")
	writeTimeout, _ := time.ParseDuration("500000ms")
	maxIdleConnDuration, _ := time.ParseDuration("1h")
	client = &fasthttp.Client{
		ReadTimeout:                   readTimeout,
		WriteTimeout:                  writeTimeout,
		MaxIdleConnDuration:           maxIdleConnDuration,
		NoDefaultUserAgentHeader:      true, // Don't send: User-Agent: fasthttp
		DisableHeaderNamesNormalizing: true, // If you set the case on your headers correctly you can enable this
		DisablePathNormalizing:        true,
		// increase DNS cache time to an hour instead of default minute
		Dial: (&fasthttp.TCPDialer{
			Concurrency:      4096,
			DNSCacheDuration: time.Hour,
		}).Dial,
		TLSConfig: &tls.Config{
			//RootCAs:            loadCACert(),
			InsecureSkipVerify: true,
		},
	}
	return client
}
//
//func (r *Request) Post(headers map[string]string, urlSuffix string, body string) []byte {
//    reqURL := r.UrlPrefix + urlSuffix
//	req := fasthttp.AcquireRequest()
//	defer fasthttp.ReleaseRequest(req)
//
//	req.SetRequestURI(reqURL)
//	req.Header.SetContentType(consts.ContentTypeJson)
//	req.Header.SetMethod(consts.POST)
//	for k, v := range headers {
//		req.Header.Set(k, v)
//	}
//	req.SetBody([]byte(body))
//
//	resp := fasthttp.AcquireResponse()
//	defer fasthttp.ReleaseResponse(resp)
//	log.Printf("Starting to Post request %s", reqURL)
//	if err := r.Client.Do(req, resp); err != nil {
//		errorInfo := fmt.Sprintf("POST error do request %s", err)
//		log.Println(errorInfo)
//		panic(errorInfo)
//	}
//	if resp.StatusCode() > 204 {
//		errorInfo := fmt.Sprintf("POST request %s failed, resp==%s", reqURL, resp.Body())
//		log.Printf(errorInfo)
//		panic(errorInfo)
//	}
//	log.Printf("Post request %s success", reqURL)
//	res := resp.Body()
//	return res
//}
//
//func (r *Request) Put(headers map[string]string, urlSuffix string, body string) []byte {
//	reqURL := r.UrlPrefix + urlSuffix
//	req := fasthttp.AcquireRequest()
//	defer fasthttp.ReleaseRequest(req)
//
//	req.SetRequestURI(reqURL)
//	req.Header.SetContentType(consts.ContentTypeJson)
//	req.Header.SetMethod(consts.PUT)
//	for k, v := range headers {
//		req.Header.Set(k, v)
//	}
//
//	req.SetBody([]byte(body))
//
//	resp := fasthttp.AcquireResponse()
//	defer fasthttp.ReleaseResponse(resp)
//	log.Printf("Starting to Put request %s", reqURL)
//	if err := r.Client.Do(req, resp); err != nil {
//		errorInfo := fmt.Sprintf("PUT request err==%s", err)
//		log.Printf(errorInfo)
//		panic(errorInfo)
//	}
//	if resp.StatusCode() > 204 {
//		errorInfo := fmt.Sprintf("PUT request %s failed, resp==%s", urlSuffix, resp.Body())
//		log.Printf(errorInfo)
//		panic(errorInfo)
//	}
//	log.Printf("Put request %s success", reqURL)
//	res := resp.Body()
//	return res
//}
//
//func (r *Request) Delete(headers map[string]string, urlSuffix string) (bool, string)  {
//	reqURL := r.UrlPrefix + urlSuffix
//	req := fasthttp.AcquireRequest()
//	defer fasthttp.ReleaseRequest(req)
//
//	req.SetRequestURI(reqURL)
//	req.Header.SetMethod(consts.DELETE)
//	req.Header.SetContentType(consts.ContentTypeJson)
//	for k, v := range headers {
//		req.Header.Set(k, v)
//	}
//
//	resp := fasthttp.AcquireResponse()
//	defer fasthttp.ReleaseResponse(resp)
//	log.Printf("Start to Delete request %s", reqURL)
//	if err := r.Client.Do(req, resp); err != nil {
//		errorInfo := fmt.Sprintf("Delete failed err==%s", err)
//	    log.Println(errorInfo)
//		panic(errorInfo)
//	}
//	res := resp.Body()
//	if resp.StatusCode() == 404 {
//		log.Printf("Delete request %s, resp==%s", urlSuffix, resp.Body())
//		return true, string(res)
//	} else if resp.StatusCode() > 204 {
//		log.Printf("Delete request %s failed, resp==%s", urlSuffix, resp.Body())
//		return false, string(res)
//	} else {
//		log.Printf("Delete request %s success", reqURL)
//		return true, ""
//	}
//}
//
//func (r *Request) Get(headers map[string]string, urlSuffix string) []byte {
//	reqURL := r.UrlPrefix + urlSuffix
//	req := fasthttp.AcquireRequest()
//	defer fasthttp.ReleaseRequest(req)
//
//	req.SetRequestURI(reqURL)
//	req.Header.SetMethod(consts.GET)
//	for k, v := range headers {
//		req.Header.Set(k, v)
//	}
//
//	resp := fasthttp.AcquireResponse()
//	defer fasthttp.ReleaseResponse(resp)
//	log.Printf("Start to Get request %s", urlSuffix)
//	if err := r.Client.Do(req, resp); err != nil {
//		log.Println("########get err", err)
//		panic("get error")
//	}
//	if resp.StatusCode() > 204 {
//		log.Printf("Get request %s failed, resp==%s", urlSuffix, resp.Body())
//		return nil
//	}
//	res := resp.Body()
//	log.Printf("Get request %s success", reqURL)
//	return res
//}
//
//
//func (r *Request) List(headers map[string]string, urlSuffix string) []byte {
//	reqURL := r.UrlPrefix + urlSuffix
//	req := fasthttp.AcquireRequest()
//	defer fasthttp.ReleaseRequest(req)
//
//	req.SetRequestURI(reqURL)
//	req.Header.SetMethod(consts.GET)
//
//	for k, v := range headers {
//		req.Header.Set(k, v)
//	}
//
//	resp := fasthttp.AcquireResponse()
//	resp.Header.SetContentType(consts.ContentTypeJson)
//	defer fasthttp.ReleaseResponse(resp)
//	log.Printf("Start to List request %s", reqURL)
//	if err := r.Client.Do(req, resp); err != nil {
//		log.Println("err==", err)
//		panic("List error")
//	}
//	if resp.StatusCode() > 202 {
//		log.Printf("List request %s failed, resp==%s", reqURL, resp.Body())
//		return nil
//	}
//	res := resp.Body()
//	log.Printf("List request %s sucess", reqURL)
//	return res
//}

func (r *Request) Patch(headers map[string]string, urlSuffix string, body string) []byte {
	reqURL := r.UrlPrefix + urlSuffix
	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)

	req.SetRequestURI(reqURL)
	req.Header.SetContentType(consts.ImagePatchJson)
	req.Header.SetMethod(consts.PATCH)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.SetBody([]byte(body))

	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)
	log.Printf("Start to Patch request %s", reqURL)
	if err := r.Client.Do(req, resp); err != nil {
		log.Println("err==", err)
		panic("Patch error do request")
	}
	if resp.StatusCode() > 204 {
		log.Printf("Patch request %s failed, resp==%s", reqURL, resp.Body())
		panic("Patch error")
	}
	log.Printf("Patch request %s sucess", reqURL)
	res := resp.Body()
	return res
}

func (r *Request) GetHeaderToken(headers map[string]string, urlSuffix string, body string) string {
	reqURL := r.UrlPrefix + urlSuffix
	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)

	req.SetRequestURI(reqURL)
	req.Header.SetContentType(consts.ContentTypeJson)
	req.Header.SetMethod(consts.POST)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	req.SetBody([]byte(body))

	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)
	log.Printf("Start to get token request %s", reqURL)
	if err := r.Client.Do(req, resp); err != nil {
		log.Println("err==", err)
		panic("post error do request")
	}
	if resp.StatusCode() > 204 {
		log.Printf("Post request %s failed, resp==%s", urlSuffix, resp.Body())
		panic("post error")
	}
	log.Printf("Post request %s sucess", urlSuffix)
	return string(resp.Header.Peek("X-Subject-Token"))
}

func (r *Request) SyncResource(resourceId string, getter getterFunc, resource map[string]interface{}) {
	var resourceBody map[string]interface{}
	if getter != nil {
		resourceBody = getter(resourceId)
	} else {
		resourceBody = resource
	}

	jsonStr, _ := json.Marshal(resourceBody)
	oldVal := cache.RedisClient.UpdateKV(resourceId, jsonStr)
	log.Println("==============sync resource success", oldVal)
}

func (r *Request) SyncMap(resourceSet, resourceId string, getter func(string) interface{}, resource interface{}) {
	var resourceBody interface{}
	if getter != nil {
		resourceBody = getter(resourceId)
	} else {
		resourceBody = resource
	}
	cache.RedisClient.SetMap(resourceSet, resourceId, resourceBody)
	log.Println("==============sync resource success")
}

func (r *Request) HandleResp(resp *fasthttp.Response) (res map[string]interface{}) {
	content := resp.Body()
	_ = json.Unmarshal(content, &res)
	return res
}

func (r *Request) HandleRespBody(resp []byte) (res map[string]interface{}) {
	_ = json.Unmarshal(resp, &res)
	return res
}

func (r *Request) DecorateResp(f func(headers map[string]string, urlSuffix string, body string) []byte) func(headers map[string]string, urlSuffix string, body string) map[string]interface{} {
    return func(headers map[string]string, urlSuffix string, body string) map[string]interface{} {
		resp := f(headers, urlSuffix, body)
		return r.HandleRespBody(resp)
	}
}

func (r *Request) DecorateGetResp(f func(headers map[string]string, urlSuffix string) []byte) func(headers map[string]string, urlSuffix string) map[string]interface{} {
	return func(headers map[string]string, urlSuffix string) map[string]interface{} {
		resp := f(headers, urlSuffix)
		return r.HandleRespBody(resp)
	}
}

// #################################################

func (r *Request) Post(headers map[string]string, urlSuffix string, body string) []byte {
	reqURL := r.UrlPrefix + urlSuffix
    req, err := http.NewRequest(consts.POST, reqURL, bytes.NewBufferString(body))
    if err != nil {
    	log.Fatalln("new request error", err)
	}
	req.Header.Set("Content-Type", consts.ContentTypeJson)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	cli := &http.Client{Timeout: 5 * 60 * time.Second}
	log.Printf("Starting to POST request %s", reqURL)
	resp, err := cli.Do(req)
	defer resp.Body.Close()
	if err != nil {
		log.Println("post err", err)
		panic("post err")
	}
	res := resp.Body
	resBody, err := ioutil.ReadAll(res)
	if err != nil {
		panic(err)
	}
	if resp.StatusCode > 204 {
		errorInfo := fmt.Sprintf("POST request %s failed, resp==%s", reqURL, resBody)
		log.Printf(errorInfo)
		panic(errorInfo)
	}
	log.Printf("Post request %s success", reqURL)

	return resBody
}

func (r *Request) Put(headers map[string]string, urlSuffix string, body string) []byte {
	reqURL := r.UrlPrefix + urlSuffix
	req, err := http.NewRequest(consts.PUT, reqURL, bytes.NewBufferString(body))
	if err != nil {
		log.Fatalln("new request error", err)
	}
	req.Header.Set("Content-Type", consts.ContentTypeJson)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	cli := &http.Client{Timeout: 5 * 60 * time.Second}
	log.Printf("Starting to PUT request %s", reqURL)
	resp, err := cli.Do(req)
	defer resp.Body.Close()
	if err != nil {
		log.Println("post err", err)
		panic("put err")
	}
	res := resp.Body
	resBody, err := ioutil.ReadAll(res)
	if err != nil {
		panic(err)
	}
	if resp.StatusCode > 204 {
		errorInfo := fmt.Sprintf("PUT request %s failed, resp==%s", urlSuffix, resBody)
		log.Printf(errorInfo)
		panic(errorInfo)
	}
	log.Printf("Put request %s success", reqURL)
	return resBody
}

func (r *Request) Delete(headers map[string]string, urlSuffix string) (bool, string)  {
	reqURL := r.UrlPrefix + urlSuffix
	req, err := http.NewRequest(consts.DELETE, reqURL, nil)
	if err != nil {
		log.Fatalln("new request error", err)
	}
	req.Header.Set("Content-Type", consts.ContentTypeJson)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	cli := &http.Client{Timeout: 5 * 60 * time.Second}
	log.Printf("Starting to DELETE request %s", reqURL)
	resp, err := cli.Do(req)
	defer resp.Body.Close()
	if err != nil {
		log.Println("delete err", err)
		panic("delete err")
	}
	res := resp.Body
	resBody, err := ioutil.ReadAll(res)
	if err != nil {
		panic(err)
	}
	if resp.StatusCode == 404 {
		log.Printf("Delete request %s, resp==%s", urlSuffix, resBody)
		return true, string(resBody)
	} else if resp.StatusCode > 204 {
		log.Printf("Delete request %s failed, resp==%s", urlSuffix, resBody)
		return false, string(resBody)
	} else {
		log.Printf("Delete request %s success", reqURL)
		return true, ""
	}
}

func (r *Request) Get(headers map[string]string, urlSuffix string) []byte {
	reqURL := r.UrlPrefix + urlSuffix
	req, err := http.NewRequest(consts.GET, reqURL, nil)
	if err != nil {
		log.Fatalln("new request error", err)
	}
	req.Header.Set("Content-Type", consts.ContentTypeJson)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	cli := &http.Client{Timeout: 5 * 60 * time.Second}
	log.Printf("Starting to GET request %s", reqURL)
	resp, err := cli.Do(req)
	defer resp.Body.Close()
	if err != nil {
		log.Println("get err", err)
		panic("get err")
	}
	res := resp.Body
	resBody, err := ioutil.ReadAll(res)
	if err != nil {
		panic(err)
	}
	if resp.StatusCode > 204 {
		log.Printf("Get request %s failed, resp==%s", urlSuffix, resBody)
		return nil
	}
	log.Printf("Get request %s success", reqURL)
	return resBody
}

func (r *Request) List(headers map[string]string, urlSuffix string) []byte {
	reqURL := r.UrlPrefix + urlSuffix
	req, err := http.NewRequest(consts.GET, reqURL, nil)
	if err != nil {
		log.Fatalln("new request error", err)
	}
	req.Header.Set("Content-Type", consts.ContentTypeJson)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	cli := &http.Client{Timeout: 5 * 60 * time.Second}
	log.Printf("Starting to LIST request %s", reqURL)
	resp, err := cli.Do(req)
	defer resp.Body.Close()
	if err != nil {
		log.Println("list err", err)
		panic("list err")
	}
	res := resp.Body
	resBody, err := ioutil.ReadAll(res)
	if err != nil {
		panic(err)
	}
	if resp.StatusCode > 202 {
		log.Printf("List request %s failed, resp==%s", reqURL, resBody)
		return nil
	}
	log.Printf("List request %s sucess", reqURL)
	return resBody
}

