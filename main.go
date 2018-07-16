package main

import (
	"database/sql"
	"flag"
	"fmt"

	"log"
	"os"
	"time"

	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/sessions"
	_ "github.com/lib/pq"
	"github.com/thinkerou/favicon"

	"encoding/gob"

	"net/http"

	"github.com/mariusor/littr.go/api"
	"github.com/mariusor/littr.go/app"
)

const defaultHost = "littr.git"
const defaultPort = 3000

var listenHost = os.Getenv("HOSTNAME")
var listenPort, _ = strconv.ParseInt(os.Getenv("PORT"), 10, 64)

var littr app.Littr

func init() {
	authKey := []byte(os.Getenv("SESS_AUTH_KEY"))
	encKey := []byte(os.Getenv("SESS_ENC_KEY"))
	if listenPort == 0 {
		listenPort = defaultPort
	}
	if listenHost == "" {
		listenHost = defaultHost
	}

	gob.Register(app.Account{})
	gob.Register(app.Flash{})
	s := sessions.NewCookieStore(authKey, encKey)
	//s.Options.Domain = listenHost
	s.Options.Path = "/"

	app.SessionStore = s

	littr = app.Littr{Host: listenHost, Port: listenPort}

	dbPw := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	dbUser := os.Getenv("DB_USER")

	connStr := fmt.Sprintf("user=%s password=%s dbname=%s sslmode=disable", dbUser, dbPw, dbName)
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Print(err)
	}

	api.Db = db
	app.Db = db
}

func main() {
	var wait time.Duration
	flag.DurationVar(&wait, "graceful-timeout", time.Second*15, "the duration for which the server gracefully wait for existing connections to finish - e.g. 15s or 1m")
	flag.Parse()

	router := gin.Default()

	router.Use(littr.Sessions)

	router.Use(favicon.New("./assets/favicon.ico"))
	router.Static("/assets", "./assets")

	router.GET("/", app.HandleIndex)

	router.GET("/submit", app.ShowSubmit)
	router.POST("/submit", app.HandleSubmit)

	router.GET("/register", app.ShowRegister)
	router.POST("/register", app.HandleRegister)

	router.GET("/~:handle", app.HandleUser)
	router.GET("/~:handle/:hash", app.HandleContent)
	router.GET("/~:handle/:hash/:direction", app.HandleVoting)

	router.GET("/parent/:hash/:parent", app.HandleParent)
	router.GET("/op/:hash/:parent", app.HandleOp)

	router.GET("/domains/:domain", app.HandleDomains)

	router.GET("/logout", app.HandleLogout)
	router.GET("/login", app.ShowLogin)
	router.POST("/login", app.HandleLogin)

	router.GET("/auth/:provider", littr.HandleAuth)
	router.GET("/auth/:provider/callback", littr.HandleCallback)

	a := router.Group("/api")
	{
		a.GET("/accounts/:handle", api.HandleAccount)
	}

	router.NoRoute(func(c *gin.Context) {
		app.HandleError(c.Writer, c.Request, http.StatusNotFound, fmt.Errorf("url %q couldn't be found", c.Request.URL))
	})

	littr.Run(router, wait)
}
