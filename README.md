# dbx

### Iterator-based [database/sql](https://pkg.go.dev/database/sql) helpers inspired by [sqlx](https://github.com/jmoiron/sqlx) 

## Run

```bash
docker compose up -d
SQLX_MYSQL_DSN=root:@tcp(localhost:33066)/dbx go test ./...
SQLX_MYSQL_DSN=root:@tcp(localhost:33066)/dbx go test -bench=. -benchmem 
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
people, err := dbx.Select[Person](ctx, db, "SELECT * FROM person").Collect()
```

```go
// collect results into a slice with specified capacity
people, err := dbx.Select[Person](ctx, db, "SELECT * FROM person LIMIT ?", limit).CollectCap(limit)
```

```go
// filter results in memory and collect into a slice
people, err := dbx.Select[Person](ctx, db, "SELECT * FROM person").
    Filter(func(p Person) bool { return p.Email != "" }).
    CollectCap(10)
```
