package main

import (
	"net/http"
	"html/template"
	"github.com/gorilla/mux"
)

type user struct {
	Email string
}

type userModel struct {
	Title string
	User user
}

// handleMain serves /~{user}
func (l *littr) handleUser(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	u := userModel{Title: vars["user"], User: user{Email: ""}}

	t, _ := template.New("user.html").ParseFiles(templateDir + "user.html")
	t.New("link.html").ParseFiles(templateDir + "content/link.html")
	t.Execute(w, u)
}