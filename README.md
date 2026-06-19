# meddevice-inventory-api

![CI](https://github.com/arpan23-27/meddevice-inventory-api/actions/workflows/ci.yml/badge.svg)

A REST API for managing medical-device stock, built in Go with PostgreSQL persistence, Redis cache-aside, and a fully Dockerised stack that comes up with a single `docker compose up`. Designed around a repository/interface architecture so the storage and cache layers are swappable without touching the HTTP layer.

> Companion frontend (Next.js dashboard) lives in `meddevice-inventory-web`.

---

## Tech stack

| Layer | Choice | Notes |
|---|---|---|
| Language | Go 1.26 | Standard library `net/http` server |
| Router | [chi/v5](https://github.com/go-chi/chi) | Path params, route groups, middleware |
| Database | PostgreSQL 16 | Accessed via `pgx/v5` connection pool — **no ORM** |
| Cache | Redis 7 | Cache-aside on single-device reads |
| Container | Docker (multi-stage) | ~12 MB final image, runs as non-root |
| Orchestration | Docker Compose | `api` + `postgres` + `redis` on one network |
| CI | GitHub Actions | `go vet` + `go test` on every push and PR |

---

## Features

- Full CRUD over `/devices` plus a `/health` endpoint.
- **Repository pattern**: handlers depend on a `DeviceRepository` interface, not a concrete store. The same handlers run against an in-memory map or PostgreSQL — switched by one line in `main.go`.
- **Cache-aside caching** of single-device reads in Redis with a TTL and delete-on-write invalidation. An `X-Cache: HIT|MISS` response header makes cache behaviour observable.
- **Pagination and category filtering** on the list endpoint, backed by a B-tree index on `category`.
- **Parameterized SQL** throughout (`$1, $2 …`) — injection-safe by construction.
- **Graceful shutdown**: drains in-flight requests on `SIGINT`/`SIGTERM` before exiting.
- **Multi-stage Docker build**: compiles in the full Go toolchain image, ships only the static binary in bare Alpine.
- **Table-driven tests** with `httptest`, runnable without any infrastructure.

---

## Architecture

```
HTTP request
   │
   ▼
chi Router ── middleware: RequestID → Logger → Recoverer
   │
   ▼
DeviceHandler                ← depends only on interfaces
   ├─► DeviceRepository (interface) ─► PostgresRepo ─► PostgreSQL
   └─► Cache (interface)            ─► RedisCache   ─► Redis
```

The handler is written against the `DeviceRepository` and `Cache` *interfaces*. Concrete implementations (`PostgresRepo`, `RedisCache`) are injected in `main.go`. This is dependency inversion in practice: swapping the entire storage engine — for example from an in-memory map to PostgreSQL — is a single assignment change, and the HTTP layer is unit-testable without a live database.

---

## Getting started

Prerequisites: Docker Desktop.

```bash
git clone https://github.com/arpan23-27/meddevice-inventory-api.git
cd meddevice-inventory-api

# bring up api + postgres + redis
docker compose up -d --build

# apply the schema (first run, or after recreating the volume)
docker compose exec -T postgres psql -U postgres -d meddevices < migrations/001_create_devices.sql

# verify
curl -s localhost:8080/health        # {"status":"ok"}
```

To run the API directly on the host (Postgres/Redis still in Docker), copy `.env.example` to `.env`, then `go run .`.

---

## API

| Method | Path | Description | Success |
|---|---|---|---|
| `GET` | `/health` | Liveness check | `200` |
| `POST` | `/devices` | Create a device | `201` |
| `GET` | `/devices` | List devices (`?category=&limit=&offset=`) | `200` |
| `GET` | `/devices/{id}` | Fetch one device | `200` / `404` |
| `PUT` | `/devices/{id}` | Update a device | `200` / `404` |
| `DELETE` | `/devices/{id}` | Delete a device | `204` / `404` |

### Sample requests

```bash
# create
curl -s -X POST localhost:8080/devices \
  -H 'Content-Type: application/json' \
  -d '{"name":"Pulse Oximeter","category":"monitoring","sku":"PX-100","quantity":40,"price":1299.0}'

# list with filter + pagination
curl -s "localhost:8080/devices?category=monitoring&limit=5&offset=0"

# fetch one (watch the cache header)
curl -i -s localhost:8080/devices/1 | grep -i x-cache   # MISS on first call, HIT after

# update (invalidates the cache key)
curl -s -X PUT localhost:8080/devices/1 \
  -H 'Content-Type: application/json' \
  -d '{"name":"Pulse Oximeter","category":"monitoring","sku":"PX-100","quantity":99,"price":1299.0}'

# delete
curl -i -s -X DELETE localhost:8080/devices/1   # 204
```

---

## How caching works

Reads use the **cache-aside** (lazy-loading) pattern:

- **Read** — check Redis first. On a **HIT**, return immediately and skip PostgreSQL. On a **MISS**, read from PostgreSQL, write the result into Redis with a 5-minute TTL, and return it. The first read of a device is therefore a MISS; subsequent reads are HITs until the TTL expires.
- **Write (update/delete)** — mutate PostgreSQL, then **delete** the Redis key (rather than overwrite it). The next read misses and repopulates from the now-current database. Delete-on-write is the simplest strategy that is always correct; the TTL is a safety net.

The cache is treated as an optimization, never a dependency: any Redis error (including a miss) degrades to a database read, so a cache failure cannot take the API down.

---

## Database

Schema is in `migrations/001_create_devices.sql`:

- `id` — `SERIAL PRIMARY KEY` (auto-increment).
- `sku` — `UNIQUE`, which also creates a supporting index.
- `category` — explicitly indexed (`idx_devices_category`) because reads filter on it.
- `price` — stored as `DOUBLE PRECISION` for simplicity. For real currency this should be `NUMERIC` or integer cents to avoid floating-point rounding; the trade-off is intentional here.

### Indexing rationale

Reads look devices up by `id` (covered by the primary key) and filter lists by `category` (covered by `idx_devices_category`). `EXPLAIN ANALYZE` confirms the planner's behaviour and how it shifts with table size:

- On a tiny table, PostgreSQL may pick a **sequential scan** — for very few rows it is cheaper than the index lookup, and the cost-based planner correctly chooses it.
- As the table grows and a filter matches a meaningful fraction of scattered rows, the plan moves to a **Bitmap Heap Scan** over `idx_devices_category`, turning random I/O into sequential I/O.
- After a bulk insert, planner row estimates can diverge sharply from actual counts until `ANALYZE` refreshes table statistics — a reminder to run `ANALYZE` after large data changes.

---

## Testing

```bash
go test ./... -v
```

Handler tests use `httptest` against the real chi router wired to the in-memory repository and a fake cache, so they require no PostgreSQL, Redis, or Docker. They cover create (including validation and malformed input), fetch (found / not-found / bad id), the cache MISS→HIT path, and list pagination/filtering. CI runs `go vet ./...` and these tests on every push and pull request.

---

## Project structure

```
meddevice-inventory-api/
├── main.go             # wiring + graceful shutdown
├── config.go           # env configuration
├── device.go           # Device model
├── repository.go       # DeviceRepository interface + ErrNotFound
├── memory_repo.go      # in-memory implementation
├── postgres_repo.go    # PostgreSQL implementation
├── db.go               # pgx connection pool
├── cache.go            # Cache interface + NoopCache
├── rediscache.go       # Redis implementation
├── handler.go          # HTTP handlers
├── handler_test.go     # table-driven httptest coverage
├── response.go         # JSON helpers
├── migrations/001_create_devices.sql
├── Dockerfile          # multi-stage build
├── docker-compose.yml  # api + postgres + redis
├── .github/workflows/ci.yml
├── .env.example
└── README.md
```

---

## Docker notes

A multi-stage build compiles the binary in `golang:1.26-alpine` and copies only the resulting static binary into `alpine:3.20`, producing a ~12 MB image with no compiler, no source, and a non-root runtime user.

Within Compose, the API reaches its dependencies by **service name** — `postgres:5432` and `redis:6379`, not `localhost`. Inside the API container `localhost` would refer to the container itself; Compose's private network resolves the other services by name. (Running the API on the host instead uses `localhost`, since the published ports live there.)

---

## Roadmap

- `v1.0.0` release tag.
- Next.js dashboard frontend (`meddevice-inventory-web`).
- Deployment to a managed host with a live demo link.
