package main

import (
	"gopkg.in/macaron.v1"
	"os"
	"net/http"
	"log"

	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"
	"github.com/markbates/goth/providers/github"
	"github.com/gorilla/sessions"
	"github.com/gorilla/securecookie"
)

func myHandler(ctx *macaron.Context) {
	ctx.Data["path"] = ctx.Req.RequestURI
    ctx.HTML(404, "404")
}

const listenHost = "myk.localdomain"

func main() {
	m := macaron.Classic()
	goth.UseProviders(
		//twitter.New(os.Getenv("TWITTER_KEY"), os.Getenv("TWITTER_SECRET"), "http://"+listenDomain+":3000/auth/twitter/callback"),
		// If you'd like to use authenticate instead of authorize in Twitter provider, use this instead.
		//twitter.NewAuthenticate(os.Getenv("TWITTER_KEY"), os.Getenv("TWITTER_SECRET"), "http://"+listenDomain+":3000/auth/twitter/callback"),

		//facebook.New(os.Getenv("FACEBOOK_KEY"), os.Getenv("FACEBOOK_SECRET"), "http://"+listenDomain+":3000/auth/facebook/callback"),
		//gplus.New(os.Getenv("GPLUS_KEY"), os.Getenv("GPLUS_SECRET"), "http://localhost:3000/auth/gplus/callback"),
		github.New(os.Getenv("GITHUB_KEY"), os.Getenv("GITHUB_SECRET"), "http://"+listenHost+":3000/auth/github/callback"),
		//gitlab.New(os.Getenv("GITLAB_KEY"), os.Getenv("GITLAB_SECRET"), "http://"+listenDomain+":3000/auth/gitlab/callback"),
	)

	m.Use(macaron.Renderer(macaron.RenderOptions{
		// Directory to load templates. Default is "templates".
		Directory: "templates",
		// Extensions to parse template files from. Defaults are [".tmpl", ".html"].
		Extensions: []string{".tmpl", ".html"},
		// Delims sets the action delimiters to the specified strings. Defaults are ["{{", "}}"].
		Delims: macaron.Delims{"{{", "}}"},
		// Appends the given charset to the Content-Type header. Default is "UTF-8".
		Charset: "UTF-8",
		// Outputs human readable JSON. Default is false.
		HTMLContentType: "text/html",
	}))

	key := securecookie.GenerateRandomKey(32)
	maxAge := 8600 * 30
	store := sessions.NewCookieStore(key)
	store.Options.Path = "/"
	store.Options.Domain = listenHost
	store.Options.HttpOnly = true
	store.Options.MaxAge = maxAge

	gothic.Store = store

	ma := make(map[string]string)
	ma["github"] = "Github"

	m.Get("/",  func(ctx *macaron.Context) {
		ctx.Data["Providers"] = ma
		ctx.HTML(200, "index")
	})

	m.Get("/auth/:provider/callback", func(ctx *macaron.Context) {
		user, err := gothic.CompleteUserAuth(ctx.Resp, ctx.Req.Request)
		if err != nil {
			ctx.Data["error"] = err
			ctx.HTML(400, "error")
			return
		}
		ctx.Data["user"] = user
		ctx.HTML(200, "user")
	})

	m.Get("/auth/:provider", func(ctx *macaron.Context) {
		user, err := gothic.CompleteUserAuth(ctx.Resp, ctx.Req.Request)
		if err != nil {
			ctx.Data["error"] = err
			ctx.HTML(400, "error")
			return
		}
		ctx.Data["user"] = user
		ctx.HTML(200, "user")
	})

	m.NotFound(myHandler)
	log.Fatal(http.ListenAndServe(listenHost+":3000", m))
}
