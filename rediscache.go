package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

func NewRedisClient(addr string) *redis.Client {
	return redis.NewClient(&redis.Options{Addr: addr})
}

type RedisCache struct {
	rdb *redis.Client
	ttl time.Duration
}

func NewRedisCache(rdb *redis.Client) *RedisCache {
	return &RedisCache{rdb: rdb, ttl: 5 * time.Minute}
}

func deviceKey(id int) string { return fmt.Sprintf("device:%d", id) }

func (c *RedisCache) Get(ctx context.Context, id int) (*Device, bool) {
	b, err := c.rdb.Get(ctx, deviceKey(id)).Bytes()
	if err != nil { // redis.Nil (miss) or any error -> treat as miss
		return nil, false
	}
	var d Device
	if json.Unmarshal(b, &d) != nil {
		return nil, false
	}
	return &d, true
}

func (c *RedisCache) Set(ctx context.Context, d *Device) {
	if b, err := json.Marshal(d); err == nil {
		c.rdb.Set(ctx, deviceKey(d.ID), b, c.ttl) // TTL = safety net
	}
}

func (c *RedisCache) Invalidate(ctx context.Context, id int) {
	c.rdb.Del(ctx, deviceKey(id)) // delete-on-write so next read repopulates
}
