package app

import (
	"fmt"

	"net/http"

	"github.com/mariusor/littr.go/models"

	"github.com/gorilla/mux"
)

// handleMain serves /p/{hash}/{parent} request
// handleMain serves /op/{hash}/{parent} request
func HandleParent(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	typ := vars["ancestor"]
	var pSel string
	if typ == "p" {
		pSel = "nlevel(cur.Path)-1"
	} else {
		pSel = " 0"
	}
	sel := fmt.Sprintf(`select par.submitted_at, par.key from content_items par 
		inner join content_items cur on subltree(cur.Path, %s, nlevel(cur.Path)) <@ par.Key::ltree
			where cur.Key ~* $1 and par.Key ~* $2`, pSel)
	rows, err := Db.Query(sel, vars["hash"], vars["parent"])
	if err != nil {
		HandleError(w, r, StatusUnknown, err)
		return
	}
	for rows.Next() {
		p := models.Content{}
		err = rows.Scan(&p.SubmittedAt, &p.Key)
		if err != nil {
			HandleError(w, r, StatusUnknown, err)
			return
		}

		url := p.PermaLink()
		http.Redirect(w, r, url, http.StatusMovedPermanently)
	}
}
