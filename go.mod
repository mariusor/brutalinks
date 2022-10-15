module github.com/mariusor/go-littr

go 1.18

require (
	git.sr.ht/~mariusor/assets v0.0.0-20220903082417-c9a1eccd9a8e
	git.sr.ht/~mariusor/wrapper v0.0.0-20221008121056-186252a01934
	github.com/go-ap/activitypub v0.0.0-20220917143152-e4e7018838c0
	github.com/go-ap/client v0.0.0-20220917143634-73d671c1b49e
	github.com/go-ap/errors v0.0.0-20220917143055-4283ea5dae18
	github.com/go-ap/jsonld v0.0.0-20220917142617-76bf51585778
	github.com/go-chi/chi/v5 v5.0.7
	github.com/google/uuid v1.3.0
	github.com/gorilla/csrf v1.7.1
	github.com/gorilla/sessions v1.2.1
	github.com/joho/godotenv v1.4.0
	github.com/mariusor/qstring v0.0.0-20200204164351-5a99d46de39d
	github.com/microcosm-cc/bluemonday v1.0.21
	github.com/openshift/osin v1.0.1
	github.com/sirupsen/logrus v1.9.0
	github.com/spacemonkeygo/httpsig v0.0.0-20181218213338-2605ae379e47
	github.com/tdewolff/minify v2.3.6+incompatible
	github.com/unrolled/render v1.5.0
	github.com/writeas/go-nodeinfo v1.0.0
	gitlab.com/golang-commonmark/markdown v0.0.0-20211110145824-bf3e522c626a
	gitlab.com/golang-commonmark/puny v0.0.0-20191124015043-9f83538fa04f
	golang.org/x/oauth2 v0.0.0-20221006150949-b44042a4b9c1
	golang.org/x/sync v0.0.0-20220929204114-8fcdb60fdcc0
	golang.org/x/text v0.3.7
)

require (
	git.sr.ht/~mariusor/go-xsd-duration v0.0.0-20220703122237-02e73435a078 // indirect
	github.com/aymerick/douceur v0.2.0 // indirect
	github.com/captncraig/cors v0.0.0-20190703115713-e80254a89df1 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/gorilla/css v1.0.0 // indirect
	github.com/gorilla/securecookie v1.1.1 // indirect
	github.com/pborman/uuid v1.2.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/tdewolff/parse v2.3.4+incompatible // indirect
	github.com/tdewolff/test v1.0.7 // indirect
	github.com/valyala/fastjson v1.6.3 // indirect
	github.com/writeas/go-webfinger v0.0.0-20190106002315-85cf805c86d2 // indirect
	gitlab.com/golang-commonmark/html v0.0.0-20191124015941-a22733972181 // indirect
	gitlab.com/golang-commonmark/linkify v0.0.0-20200225224916-64bca66f6ad3 // indirect
	gitlab.com/golang-commonmark/mdurl v0.0.0-20191124015652-932350d1cb84 // indirect
	golang.org/x/net v0.0.0-20221004154528-8021a29435af // indirect
	golang.org/x/sys v0.0.0-20221006211917-84dc82d7e875 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/protobuf v1.28.1 // indirect
	gopkg.in/yaml.v3 v3.0.0-20200615113413-eeeca48fe776 // indirect
)

replace github.com/gorilla/sessions => github.com/mariusor/sessions v1.2.2-0.20211229142436-b33eb696f35b

replace github.com/unrolled/render => github.com/mariusor/render v1.5.1-0.20221015183938-0945c87146be
