package main

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepo struct{ pool *pgxpool.Pool }

func NewPostgresRepo(pool *pgxpool.Pool) *PostgresRepo { return &PostgresRepo{pool: pool} }

func (r *PostgresRepo) Create(ctx context.Context, d *Device) error {
	return r.pool.QueryRow(ctx,
		`INSERT INTO devices (name, category, sku, quantity, price)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id, updated_at`,
		d.Name, d.Category, d.SKU, d.Quantity, d.Price,
	).Scan(&d.ID, &d.UpdatedAt)
}

func (r *PostgresRepo) GetByID(ctx context.Context, id int) (*Device, error) {
	var d Device
	err := r.pool.QueryRow(ctx,
		`SELECT id, name, category, sku, quantity, price, updated_at FROM devices WHERE id = $1`, id,
	).Scan(&d.ID, &d.Name, &d.Category, &d.SKU, &d.Quantity, &d.Price, &d.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound // translate driver error to domain error
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *PostgresRepo) List(ctx context.Context, category string, limit, offset int) ([]Device, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, category, sku, quantity, price, updated_at
		 FROM devices
		 WHERE ($1 = '' OR category = $1)
		 ORDER BY id LIMIT $2 OFFSET $3`, category, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Device
	for rows.Next() {
		var d Device
		if err := rows.Scan(&d.ID, &d.Name, &d.Category, &d.SKU, &d.Quantity, &d.Price, &d.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *PostgresRepo) Update(ctx context.Context, d *Device) error {
	err := r.pool.QueryRow(ctx,
		`UPDATE devices SET name=$1, category=$2, sku=$3, quantity=$4, price=$5, updated_at=NOW()
		 WHERE id=$6 RETURNING updated_at`,
		d.Name, d.Category, d.SKU, d.Quantity, d.Price, d.ID,
	).Scan(&d.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func (r *PostgresRepo) Delete(ctx context.Context, id int) error {
	ct, err := r.pool.Exec(ctx, `DELETE FROM devices WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}