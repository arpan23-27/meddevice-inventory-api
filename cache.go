package main 

import "context"

type Cache interface {
	Get(ctx  context.Context, id int) (*Device, bool)
	Set(ctx context.Context, d *Device)
	Invalidate(ctx context.Context, id int)
}

type NoopCache struct{}


func (NoopCache) Get(context.Context, int) (*Device, bool) { return nil, false }
func (NoopCache) Set(context.Context, *Device)             {}
func (NoopCache) Invalidate(context.Context, int)          {}