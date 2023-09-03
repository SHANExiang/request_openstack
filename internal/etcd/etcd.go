package etcd

import (
	"context"
	"go.etcd.io/etcd/client/v3"
	"log"
	"time"
)

var Client *clientv3.Client

func InitEtcd()  {
	var err error
    Client, err = clientv3.New(clientv3.Config{
        Endpoints: []string{"10.50.114.157:2379"},
        DialTimeout: 5 * time.Second,
	})
    if err != nil {
    	log.Printf("Connect to etcd server error:%v\n", err)
	}
	log.Println("Connect to etcd server success")
}

func GetContext(sec int64) context.Context {
	ctx, _ := context.WithTimeout(context.Background(), time.Duration(sec) * time.Second)
	return ctx
}

func PutKV(key, value string, opts ...clientv3.OpOption) {
    ctx := GetContext(10)
    resp, err := Client.Put(ctx, key, value, opts...)
    if err != nil {
    	log.Println("Put to etcd server failed", err)
	}
	log.Println("Put to etcd server success, resp==", resp)
}

func GetKV(key string) string {
	ctx := GetContext(10)
	resp, err := Client.Get(ctx, key)
	if err != nil {
		log.Println("Get from etcd server failed", err)
		panic("")
	}
	kvs := resp.Kvs
	log.Println("Get from etcd server success, resp==", resp.Kvs)
	for _, v := range kvs {
		return string(v.Value)
	}
	return ""
}

func GrantLease(expireTime int64) *clientv3.LeaseGrantResponse {
	ctx := GetContext(100)
	g, err := Client.Grant(ctx, expireTime)
	if err != nil {
		log.Println("Grant lease failed", err)
		panic("")
	}
	log.Println("Grant lease success, resp==", g)
	return g
}

func DeleteKV(key string) {
	ctx := GetContext(10)
	resp, err := Client.Delete(ctx, key)
	if err != nil {
		log.Println("Delete from etcd server failed", err)
		panic("")
	}
	log.Println("Delete from etcd server success, resp==", resp)
}
