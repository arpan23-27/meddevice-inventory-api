package main 

import (
	"context"
	"errors"
)

var  ErrNotFound = errors.New("device not found")

type DeviceRepository  interface {
	Create(ctx  context.Context, d *Device) error
	GetByID(ctx context.Context, id int) (*Device, error)
	List(ctx context.Context, category string, limit, offset int) ([]Device, error)
	Update(ctx context.Context, d *Device) error
	Delete(ctx context.Context, id int) error
}