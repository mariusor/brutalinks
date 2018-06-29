package main

import (
	"github.com/astaxie/beego/orm"
	"github.com/gorilla/mux"
	"net/http"
)

// handleMain serves /parent/{hash} request
func (l *littr) handleParent(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	db, err := orm.GetDB("default")
	if err != nil {
		l.handleError(w, r, err)
		return
	}

	sel := `select par.submitted_at, par.key from content_items par 
		inner join content_items cur on subltree(cur.Path, nlevel(cur.Path)-1, nlevel(cur.Path)) <@ par.Key::ltree
			where cur.Key ~* $1 and par.Key ~* $2`
	rows, err := db.Query(sel, vars["hash"], vars["parent"])
	if err != nil {
		l.handleError(w, r, err)
		return
	}
	for rows.Next() {
		p := Content{}
		err = rows.Scan(&p.SubmittedAt, &p.Key)
		if err != nil {
			l.handleError(w, r, err)
			return
		}

		url := p.PermaLink()
		http.Redirect(w, r, url, http.StatusMovedPermanently)
	}
}
