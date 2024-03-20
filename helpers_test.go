package dbx

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const personRecordNum = 20

var (
	mysqldb *sql.DB
	active  []*sql.DB
)

func init() {
	time.Local = time.UTC
	ConnectAll()
}

type Emailer struct {
	Email string `db:"email"`
}

type Person struct {
	FirstName string    `db:"first_name"` // 0
	LastName  string    `db:"last_name"`  // 1
	AddedAt   time.Time `db:"added_at"`   // 3
	Emailer             // 2
}

func person(index int) Person {
	i := strconv.Itoa(index)
	return Person{
		FirstName: "FirstName" + i,
		LastName:  "LastName" + i,
		AddedAt:   time.Now(),
		Emailer: Emailer{
			Email: i + "@domain.com",
		},
	}
}

func MultiExecContext(ctx context.Context, e Execer, query string) {
	stmts := strings.Split(query, ";\n")
	if len(strings.Trim(stmts[len(stmts)-1], " \n\t\r")) == 0 {
		stmts = stmts[:len(stmts)-1]
	}
	for _, s := range stmts {
		_, err := e.ExecContext(ctx, s)
		if err != nil {
			fmt.Println(err, s)
		}
	}
}

func RunWithSchemaContext(ctx context.Context, schema Schema, t testing.TB, test func(ctx context.Context, db *sql.DB, t testing.TB)) {
	runner := func(ctx context.Context, db *sql.DB, t testing.TB, create, drop, now string) {
		defer func() {
			MultiExecContext(ctx, db, drop)
		}()

		MultiExecContext(ctx, db, create)
		test(ctx, db, t)
	}

	create, drop, now := schema.MySQL()
	runner(ctx, mysqldb, t, create, drop, now)
}

func Open(driverName, dataSourceName string) (*sql.DB, error) {
	db, err := sql.Open(driverName, dataSourceName)
	if err != nil {
		return nil, err
	}
	return db, nil
}

func Connect(driverName, dataSourceName string) (*sql.DB, error) {
	db, err := Open(driverName, dataSourceName)
	if err != nil {
		return nil, err
	}
	err = db.Ping()
	if err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func ConnectAll() {
	var err error
	mydsn := os.Getenv("SQLX_MYSQL_DSN")

	if mydsn == "skip" {
		fmt.Println("Skipping MySQL tests")
		return
	}

	if !strings.Contains(mydsn, "parseTime=true") {
		mydsn += "?parseTime=true"
	}

	mysqldb, err = Connect("mysql", mydsn)
	if err != nil {
		fmt.Printf("Disabling MySQL tests:\n    %v", err)
	}
}

func loadDefaultFixtureContext(ctx context.Context, db *sql.DB, t testing.TB) {
	t.Helper()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	for i := range personRecordNum {
		ind := strconv.Itoa(i + 1)
		_, _ = tx.ExecContext(ctx, "INSERT INTO person (first_name, last_name, email) VALUES (?, ?, ?)",
			"FirstName"+ind, "LastName"+ind, ind+"@domain.com")
	}

	tx.ExecContext(ctx, "INSERT INTO place (country, city, telcode) VALUES (?, ?, ?)", "United States", "New York", "1")
	tx.ExecContext(ctx, "INSERT INTO place (country, telcode) VALUES (?, ?)", "Hong Kong", "852")
	tx.ExecContext(ctx, "INSERT INTO place (country, telcode) VALUES (?, ?)", "Singapore", "65")

	// TODO 2024/02/24 @Jimeux bind type setting?
	/*	if db.DriverName() == "mysql" {
			tx.ExecContext(ctx, tx.Rebind("INSERT INTO capplace (`COUNTRY`, `TELCODE`) VALUES (?, ?)"), "Sarf Efrica", "27")
		} else {
			tx.ExecContext(ctx, tx.Rebind("INSERT INTO capplace (\"COUNTRY\", \"TELCODE\") VALUES (?, ?)"), "Sarf Efrica", "27")
		}*/
	tx.ExecContext(ctx, "INSERT INTO employees (name, id) VALUES (?, ?)", "Peter", "4444")
	tx.ExecContext(ctx, "INSERT INTO employees (name, id, boss_id) VALUES (?, ?, ?)", "Joe", "1", "4444")
	tx.ExecContext(ctx, "INSERT INTO employees (name, id, boss_id) VALUES (?, ?, ?)", "Martin", "2", "4444")
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

type Schema struct {
	create string
	drop   string
}

func (s Schema) MySQL() (string, string, string) {
	return strings.Replace(s.create, `"`, "`", -1), s.drop, `now()`
}

var defaultSchema = Schema{
	create: `
CREATE TABLE person (
	first_name text,
	last_name text,
	email text,
	added_at timestamp default now()
);

CREATE TABLE place (
	country text,
	city text NULL,
	telcode integer
);

CREATE TABLE capplace (
	"COUNTRY" text,
	"CITY" text NULL,
	"TELCODE" integer
);

CREATE TABLE nullperson (
    first_name text NULL,
    last_name text NULL,
    email text NULL
);

CREATE TABLE employees (
	name text,
	id integer,
	boss_id integer
);

`,
	drop: `
drop table person;
drop table place;
drop table capplace;
drop table nullperson;
drop table employees;
`,
}
