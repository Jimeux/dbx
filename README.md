# dbx

![Build](https://github.com/Jimeux/dbx/actions/workflows/main.yml/badge.svg)

### Iterator-based [database/sql](https://pkg.go.dev/database/sql) helpers based on [sqlx](https://github.com/jmoiron/sqlx) 

## Run

```bash
docker compose up -d
GOEXPERIMENT=rangefunc SQLX_MYSQL_DSN=root:@tcp(localhost:33066)/dbx go test ./...
GOEXPERIMENT=rangefunc SQLX_MYSQL_DSN=root:@tcp(localhost:33066)/dbx go test -bench=. -benchmem 
```
