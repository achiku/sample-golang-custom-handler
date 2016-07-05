// https://joeshaw.org/net-context-and-http-handler/
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"golang.org/x/net/context"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/rs/xhandler"
	"github.com/rs/xmux"
)

const (
	ctxTxKey = "tx"
)

// AppConfig application config
type AppConfig struct {
	DBURI      string
	DBPort     string
	DBUserName string
	DBName     string
	ServerPort string
	AppName    string
}

// App application
type App struct {
	Name   string
	Config AppConfig
	DB     *sqlx.DB
}

func (ap *App) loggingMiddleware(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		t1 := time.Now()
		next.ServeHTTP(w, r)
		t2 := time.Now()
		log.Printf("[%s] [%s] %q %v\n", ap.Name, r.Method, r.URL.String(), t2.Sub(t1))
	}
	return http.HandlerFunc(fn)
}

func (ap *App) recoverMiddleware(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("panic: %s\n", err)
				w.Header().Set("Content-type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				res := map[string]string{"message": "internal error"}
				json.NewEncoder(w).Encode(res)
			}
		}()
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}

// NewApp return application
func NewApp() (*App, error) {
	cfg := AppConfig{
		DBURI:      "localhost",
		DBPort:     "5432",
		DBUserName: "pgtest",
		DBName:     "pgtest",
		ServerPort: "8991",
	}
	db, err := sqlx.Connect("postgres",
		fmt.Sprintf("user=%s dbname=%s sslmode=disable", cfg.DBUserName, cfg.DBName))

	if err != nil {
		return nil, err
	}
	app := &App{
		Name:   "app",
		Config: cfg,
		DB:     db,
	}
	return app, nil
}

// EchoApp ping if server alive
type EchoApp struct {
	*App
	Name string
}

// DBApp ping if server alive
type DBApp struct {
	*App
	Name string
}

// AppHandlerC app handler
type AppHandlerC func(context.Context, http.ResponseWriter, *http.Request) (int, error)

// ServeHTTPC serve http with context
func (ah AppHandlerC) ServeHTTPC(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	status, err := ah(ctx, w, r)
	if err != nil {
		http.Error(w, err.Error(), status)
	}
}

// TransactionHandlerC app handler
type TransactionHandlerC struct {
	*App
	H func(context.Context, http.ResponseWriter, *http.Request) (int, error)
}

// ServeHTTPC serve http with context
func (ah TransactionHandlerC) ServeHTTPC(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	tx, err := ah.App.DB.Begin()
	defer tx.Rollback()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	ctx = context.WithValue(ctx, ctxTxKey, tx)
	status, err := ah.H(ctx, w, r)
	if err != nil {
		http.Error(w, err.Error(), status)
	}
}

func getTx(ctx context.Context) *sql.Tx {
	return ctx.Value(ctxTxKey).(*sql.Tx)
}

// EchoServer ping server
func (ap *EchoApp) EchoServer(ctx context.Context, w http.ResponseWriter, r *http.Request) (int, error) {
	fmt.Fprintf(w, "hello, server!")
	return http.StatusOK, nil
}

// EchoDatabase ping server and database
func (ap *EchoApp) EchoDatabase(ctx context.Context, w http.ResponseWriter, r *http.Request) (int, error) {
	tx := getTx(ctx)
	var t time.Time
	err := tx.QueryRow("SELECT now()").Scan(&t)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	msg := fmt.Sprintf("hello, database! at %s", t)
	fmt.Fprintf(w, msg)
	return http.StatusOK, nil
}

func helloctx(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "hello, world!")
	return
}

func helloret(ctx context.Context, w http.ResponseWriter, r *http.Request) (int, error) {
	fmt.Fprintf(w, "hello, world!")
	return http.StatusOK, nil
}

func hellotran(ctx context.Context, w http.ResponseWriter, r *http.Request) (int, error) {
	tx := getTx(ctx)
	var t time.Time
	err := tx.QueryRow("SELECT now()").Scan(&t)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	msg := fmt.Sprintf("hello, database! at %s", t)
	fmt.Fprintf(w, msg)
	return http.StatusOK, nil
}

func tranSelect(ctx context.Context, w http.ResponseWriter, r *http.Request) (int, error) {
	tx := getTx(ctx)
	var t time.Time
	err := tx.QueryRow("SELECT now()").Scan(&t)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	fmt.Fprintf(w, "hello, from database at %s !", t)
	return http.StatusOK, nil
}

func (app *DBApp) notranSelect(ctx context.Context, w http.ResponseWriter, r *http.Request) (int, error) {
	var t time.Time
	if err := app.DB.QueryRow(`SELECT now()`).Scan(&t); err != nil {
		return http.StatusInternalServerError, err
	}
	fmt.Fprintf(w, "hello, from database at %s !", t)
	return http.StatusOK, nil
}

func main() {
	app, err := NewApp()
	if err != nil {
		log.Fatal(err)
	}

	c := xhandler.Chain{}
	c.Use(app.recoverMiddleware)
	c.Use(app.loggingMiddleware)

	mux := xmux.New()
	api := mux.NewGroup("/api")

	echoApp := EchoApp{App: app, Name: "echo"}
	dbApp := DBApp{App: app, Name: "db"}
	api.GET("/echo/server", AppHandlerC(echoApp.EchoServer))
	api.GET("/echo/database", TransactionHandlerC{App: app, H: echoApp.EchoDatabase})
	api.GET("/hello/context1", xhandler.HandlerFuncC(helloctx))
	api.GET("/hello/context2", AppHandlerC(helloret))
	api.GET("/hello/context3", TransactionHandlerC{App: app, H: hellotran})
	api.GET("/select/tran", TransactionHandlerC{App: app, H: tranSelect})
	api.GET("/select/notran", AppHandlerC(dbApp.notranSelect))

	rootCtx := context.Background()
	if err := http.ListenAndServe(":"+app.Config.ServerPort, c.HandlerCtx(rootCtx, mux)); err != nil {
		log.Fatal(err)
	}
}
