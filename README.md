# dbx

![Build](https://github.com/Jimeux/dbx/actions/workflows/main.yml/badge.svg)

### Iterator-based [database/sql](https://pkg.go.dev/database/sql) helpers based on [sqlx](https://github.com/jmoiron/sqlx) 

## Run

```bash
docker compose up -d
GOEXPERIMENT=rangefunc SQLX_MYSQL_DSN=root:@tcp(localhost:33066)/dbx go test ./...
GOEXPERIMENT=rangefunc SQLX_MYSQL_DSN=root:@tcp(localhost:33066)/dbx go test -bench=. -benchmem 
```

## Examples

```go
// loop through iterator directly to populate a map
byName := make(map[string]Person)
for p, err := range dbx.Select[Person](ctx, db, "SELECT * FROM person") {
    if err != nil {
        return
    }
    byName[p.FirstName] = p
}
```

```go
// collect results into a slice
people, err := dbx.Select[Person](ctx, db, "SELECT * FROM person").Slice()
```

```go
// collect results into a slice with specified capacity
people, err := dbx.Select[Person](ctx, db, "SELECT * FROM person LIMIT ?", limit).SliceCap(limit)
```

```go
// filter results in memory and collect into a slice
people, err := dbx.Select[Person](ctx, db, "SELECT * FROM person").
    Filter(func(p Person) bool { return p.Email != "" }).
    SliceCap(10)
```
