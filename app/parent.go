package app

import (
	"net/http"

	"github.com/mariusor/littr.go/models"

	"github.com/go-chi/chi"
)

// HandleParent serves /parent/{hash}/{parent} request
func HandleParent(w http.ResponseWriter, r *http.Request) {
	sel := `select accounts.handle, par.key from content_items par 
		inner join content_items cur on subltree(cur.Path, nlevel(cur.Path)-1, nlevel(cur.Path)) <@ par.Key::ltree
		inner join accounts on accounts.id = par.submitted_by
			where cur.Key ~* $1 and par.Key ~* $2`
	rows, err := Db.Query(sel, chi.URLParam(r, "hash"), chi.URLParam(r, "parent"))
	if err != nil {
		HandleError(w, r, StatusUnknown, err)
		return
	}
	for rows.Next() {
		p := models.Content{}
		var handle string
		err = rows.Scan(&handle, &p.Key)
		if err != nil {
			HandleError(w, r, StatusUnknown, err)
			return
		}

		url := PermaLink(p, handle)
		Redirect(w, r, url, http.StatusMovedPermanently)
	}
}

// HandleOp serves /op/{hash}/{parent} request
func HandleOp(w http.ResponseWriter, r *http.Request) {
	sel := `select accounts.handle, par.key from content_items par 
		inner join content_items cur on subltree(cur.Path, 0, nlevel(cur.Path)) <@ par.Key::ltree
		inner join accounts on accounts.id = par.submitted_by
			where cur.Key ~* $1 and par.Key ~* $2`
	rows, err := Db.Query(sel, chi.URLParam(r, "hash"), chi.URLParam(r, "parent"))
	if err != nil {
		HandleError(w, r, StatusUnknown, err)
		return
	}
	for rows.Next() {
		p := models.Content{}
		var handle string
		err = rows.Scan(&handle, &p.Key)
		if err != nil {
			HandleError(w, r, StatusUnknown, err)
			return
		}

		it := LoadItem(p)
		Redirect(w, r, it.PermaLink(), http.StatusMovedPermanently)
	}
}
