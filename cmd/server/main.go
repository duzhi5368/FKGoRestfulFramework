package main

import(
	"flag"
	"os"
	"fmt"
	"time"
	"context"
	"database/sql"
	"net/http"

	"github.com/go-ozzo/ozzo-dbx"
	_ "github.com/go-sql-driver/mysql"

	"github.com/go-ozzo/ozzo-routing/v2"
	"github.com/go-ozzo/ozzo-routing/v2/content"
	"github.com/go-ozzo/ozzo-routing/v2/cors"

	"pkg/log"
	"pkg/accesslog"
	"pkg/dbcontext"

	"local/config"
	_ "local/album"
	_ "local/auth"
	"local/healthcheck"
	"local/errors"
	"local/controller"
)

var Version = "1.0.0"
var AppConfig = flag.String("config", "./config/dev.yml", "path to the config file")

func main(){
	// parse command line args.
	flag.Parse()
	fmt.Printf("args=%s, num=%d\n", flag.Args(), flag.NArg())
    for i := 0; i != flag.NArg(); i++ {
        fmt.Printf("arg[%d] = %s\n", i, flag.Arg(i))
    }
	// create logger with server's version.
	logger := log.New().With(nil, "version", Version)
	logger.Info("server init...")

	// load application's configurations.
	cfg, err := config.Load(*AppConfig, logger)
	if err != nil {
		logger.Errorf("failed to load application configuration: %s", err)
		os.Exit(-1)
	}

	// connect to the database.
	db, err := dbx.MustOpen("mysql", cfg.DSN)
	if err != nil {
		logger.Errorf("failed to connect database: %s", err)
		os.Exit(-1)
	}
	// registe callback funcions.
	db.QueryLogFunc = logDBQuery(logger)
	db.ExecLogFunc = logDBExec(logger)
	// registe to close database's connect.
	defer func() {
		if err := db.Close(); err != nil {
			logger.Error(err)
		}
	}()

	// create HTTP server.
	address := fmt.Sprintf(":%v", cfg.ServerPort)
	hs := &http.Server{
		Addr:    address,
		Handler: HTTPHandler(logger, dbcontext.New(db), cfg),
	}

	// start HTTP server and registe for shutdown.
	go routing.GracefulShutdown(hs, 10*time.Second, logger.Infof)
	logger.Infof("server %v is running at %v", Version, address)

	if err := hs.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error(err)
		os.Exit(-1)
	}
}

func HTTPHandler(logger log.Logger, db *dbcontext.DB, cfg *config.Config) http.Handler {
	router := routing.New()
	router.Use(
		accesslog.Handler(logger),
		errors.Handler(logger),
		content.TypeNegotiator(content.JSON),
		cors.Handler(cors.AllowAll),
	)

	// register health check handler.
	// if we want add more handlers with no groups, pls see ref: internal/healthcheck/api.go
	healthcheck.RegisterHandlers(router, Version)

	// create v1 router group
	rg_v1 := router.Group("/v1")

	/* if you need JWT auth, open this comment
	authHandler := auth.Handler(cfg.JWTSigningKey)
	album.RegisterHandlers(rg_v1.Group(""),
		album.NewService(album.NewRepository(db, logger), logger),
		authHandler, logger,
	)
	auth.RegisterHandlers(rg_v1.Group(""),
		auth.NewService(cfg.JWTSigningKey, cfg.JWTExpiration, logger),
		logger,
	)
	*/

	// my core http msg handler code.
	contoller.RegisterLoginHandlers(rg_v1.Group(""), logger, db)


	/* test code
	rg_v1.Get("/test1", func(c *routing.Context) error {
		return c.Write("GET example")
	})
	rg_v1.Get("/test2/<id>", func (c *routing.Context) error {
		fmt.Fprintf(c.Response, "ID: %v", c.Param("id"))
		return c.Write("example with params")
	})
	rg_v1.Post("/test3", func(c *routing.Context) error {
		data := &struct{
			A string
			B bool
		}{}
		// assume the body data is: {"A":"abc", "B":true}
		// data will be populated as: {A: "abc", B: true}
		if err := c.Read(&data); err != nil {
			return err
		}
		return c.Write("POST example")
	})
	// only accept numbers id.
	rg_v1.Put(`/test4/<id:\d+>`, func(c *routing.Context) error {
		return c.Write("example with limit params: " + c.Param("id"))
	})
	*/


	return router
}



// logDBQuery returns a logging function that can be used to log SQL queries.
func logDBQuery(logger log.Logger) dbx.QueryLogFunc {
	return func(ctx context.Context, t time.Duration, sql string, rows *sql.Rows, err error) {
		if err == nil {
			logger.With(ctx, "duration", t.Milliseconds(), "sql", sql).Info("DB query successful")
		} else {
			logger.With(ctx, "sql", sql).Errorf("DB query error: %v", err)
		}
	}
}

// logDBExec returns a logging function that can be used to log SQL executions.
func logDBExec(logger log.Logger) dbx.ExecLogFunc {
	return func(ctx context.Context, t time.Duration, sql string, result sql.Result, err error) {
		if err == nil {
			logger.With(ctx, "duration", t.Milliseconds(), "sql", sql).Info("DB execution successful")
		} else {
			logger.With(ctx, "sql", sql).Errorf("DB execution error: %v", err)
		}
	}
}