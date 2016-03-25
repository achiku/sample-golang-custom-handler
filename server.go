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

func (ap *App) transactionMiddleware() func(next xhandler.HandlerC) xhandler.HandlerC {
	return func(next xhandler.HandlerC) xhandler.HandlerC {
		fn := func(ctx context.Context, w http.ResponseWriter, r *http.Request) {
			tx, err := ap.DB.Begin()
			if err != nil {
				log.Fatal(err)
			}
			ctx = context.WithValue(ctx, ctxTxKey, tx)
			log.Println(w.Header())
			next.ServeHTTPC(ctx, w, r)
			w.Header().Set("Request-id", "Request ids!")
			log.Println(w.Header())
			tx.Commit()
		}
		return xhandler.HandlerFuncC(fn)
	}
}

func (ap *App) jsonResponseMiddleware(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-type", "application/json")
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}

func getTx(ctx context.Context) *sql.Tx {
	return ctx.Value(ctxTxKey).(*sql.Tx)
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

// HandlerResult struct
type HandlerResult struct {
	StatusCode int
	Message    string
	Error      error
}

// EchoApp ping if server alive
type EchoApp struct {
	*App
	Name string
}

func (ap *App) sendSimpleErrorMessage(w http.ResponseWriter, msg string) {
	res := map[string]string{"message": msg}
	w.WriteHeader(http.StatusInternalServerError)
	json.NewEncoder(w).Encode(res)
}

func (ap *App) sendSimpleMessage(w http.ResponseWriter, msg string) {
	res := map[string]string{"message": msg}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(res)
}

// EchoServer ping server
func (ap *EchoApp) EchoServer(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	msg := fmt.Sprintf("hello, server!")
	ap.sendSimpleMessage(w, msg)
	return
}

// EchoDatabase ping server and database
func (ap *EchoApp) EchoDatabase(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	tx := getTx(ctx)
	var t time.Time
	err := tx.QueryRow("SELECT now()").Scan(&t)
	if err != nil {
		ap.sendSimpleErrorMessage(w, "failed to access db")
		return
	}
	msg := fmt.Sprintf("hello, database! at %s", t)
	w.Header().Set("EchoDatabase", "yes")
	w.WriteHeader(http.StatusOK)
	log.Println(w.Header())
	ap.sendSimpleMessage(w, msg)
	return
}

func main() {
	app, err := NewApp()
	if err != nil {
		log.Fatal(err)
	}

	c := xhandler.Chain{}
	c.Use(app.recoverMiddleware)
	c.Use(app.loggingMiddleware)
	c.Use(app.jsonResponseMiddleware)
	c.UseC(app.transactionMiddleware())

	mux := xmux.New()
	api := mux.NewGroup("/api")

	echoApp := EchoApp{App: app, Name: "echo"}
	api.GET("/echo/server", xhandler.HandlerFuncC(echoApp.EchoServer))
	api.GET("/echo/database", xhandler.HandlerFuncC(echoApp.EchoDatabase))

	rootCtx := context.Background()
	if err := http.ListenAndServe(":"+app.Config.ServerPort, c.HandlerCtx(rootCtx, mux)); err != nil {
		log.Fatal(err)
	}
}
