package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/redis/go-redis/v9"
	"log"
	"time"
)

var RedisClient *Redis

type Redis struct {
	*redis.ClusterClient
	ctx                           context.Context
}


func NewClusterClient() {
	ctx := context.Background()
	RedisClient = &Redis{
		redis.NewClusterClient(
			&redis.ClusterOptions{
				Addrs: []string{
					"10.50.114.157:6371",
					"10.50.114.157:6372",
					"10.50.114.157:6373",
					"10.50.114.157:6374",
					"10.50.114.157:6375",
					"10.50.114.157:6376",
				},
				Password: "123456",
				DialTimeout: 50 * time.Second,
				ReadTimeout: 50 * time.Second,
				WriteTimeout: 50 * time.Second,
		}),
		ctx,
	}
	pong, err := RedisClient.Ping(ctx).Result()
	if err != nil {
		log.Println("connect redis failed", err)
		panic("")
	}
	log.Println("pong==", pong)
}

func (r *Redis) Exist(key string) bool {
	exist, err := r.Exists(r.ctx, key).Result()
	if err != nil {
		log.Println("exist exec failed err==", err)
		panic("")
	}
	log.Println("exist exec success res==", exist)
	return exist == 1
}

func (r *Redis) DeleteKV(key string)  {
	res, err := r.Del(r.ctx, key).Result()
	if err != nil {
		log.Println("delete from redis failed", key, err)
	} else {
		log.Println("delete from redis success", key, res)
	}
}

func (r *Redis) GetMembers(key string) []string {
	members, err := r.SMembers(r.ctx, key).Result()
	if err != nil {
		log.Println("get from redis failed", err)
		panic("")
	}
	return members
}

func (r *Redis) GetVal(key string) string {
	val, err := r.Get(r.ctx, key).Result()
	if err != nil {
		log.Println("get from redis failed", err)
		panic("")
	}
	return val
}

func (r *Redis) UpdateKV(key string, val interface{}) string {
    oldVal, err := r.GetSet(r.ctx, key, val).Result()
    if err != nil {
    	log.Println("update key failed", err)
    	panic("")
	}
	return oldVal
}

func (r *Redis) RemoveFromSlice(key, val string) {
	res, err := r.SRem(r.ctx, key, val).Result()
	if err != nil {
		log.Println("removeFromSlice exec failed err==", err)
		panic("")
	}
	log.Println("removeFromSlice exec success res==", res)
}

func (r *Redis) IsMemberOfSlice(key, val string) bool {
	exist, err := r.SIsMember(r.ctx, key, val).Result()
	if err != nil {
		log.Println("isMemberOfSlice exec failed err==", err)
		panic("")
	}
	return exist
}

func (r *Redis) AddSliceAndJson(resourceId, resourceSet string, resourceBody map[string]interface{})  {
	jsonStr, _ := json.Marshal(resourceBody)
	result := r.Set(r.ctx, resourceId, jsonStr, -1)
	HandleStatusCmd(result)

	result2 := r.SAdd(r.ctx, resourceSet, resourceId)
	HandleIntCmd(result2)
}

func (r *Redis) SetMap(resourceSet, resourceId string, resourceBody interface{}) {
	jsonStr, _ := json.Marshal(resourceBody)
	err := r.HSet(r.ctx, resourceSet, resourceId, jsonStr).Err()
	if err != nil {
		log.Println("set map failed", err)
		return
	}
	log.Println("set map success", resourceId)
}

func (r *Redis) DeleteMap(resourceSet, resourceId string) {
	err := r.HDel(r.ctx, resourceSet, resourceId).Err()
	if err != nil {
		log.Println("delete map failed", err)
		return
	}
	log.Println("delete map success", resourceId)
}

func (r *Redis) GetMaps(resourceSet string) []string {
	res, err := r.HKeys(r.ctx, resourceSet).Result()
	if err != nil {
		log.Println("get map failed", err)
		panic("")
	}
	log.Println(fmt.Sprintf("get %s success %s", resourceSet, res))
	return res
}

func (r *Redis) ExistMap(resourceSet, resourceId string) bool {
	res, err := r.HExists(r.ctx, resourceSet, resourceId).Result()
	if err != nil {
		log.Println("exist map failed", err)
		panic("")
	}
	log.Println("exist map success", res)
	return res
}

func (r *Redis) GetMap(resourceSet, resourceId string) string {
	res, err := r.HGet(r.ctx, resourceSet, resourceId).Result()
	if err != nil {
		log.Println("get map failed", err)
		panic("")
	}
	log.Println("get map success", res)
	return res
}

func HandleStatusCmd(result *redis.StatusCmd) {
	res2, err := result.Result()
	if err != nil {
		log.Println("redis set failed", err)
		return
	}
	log.Println("redis set success", res2)
}

func HandleIntCmd(result *redis.IntCmd) {
	res2, err := result.Result()
	if err != nil {
		log.Println("redis set failed", err)
		return
	}
	log.Println("redis set success", res2)
}
