## The "containers all the way down" release

This month's updates for the projects in the #fedbox, the generic #ActivityPub service, ecosystem:

* I brought the container image for fedbox to a working state. 
It can be fetched from [Quay.io](https://quay.io/repository/fedbox/fedbox) 

* I renamed the **littr.go** repository to [go-littr](https://github.com/mariusor/go-littr) to avoid the problems the go tooling has 
with projects whose name end in `.go`, but still keep the pun on which the original name was based.
Special thanx to [@muesli](https://github.com/muesli) that [brought](https://github.com/mariusor/go-littr/issues/30) this to my attention.

* I added another storage backend on top of [badger](github.com/dgraph-io/badger/v2).

* I have split the [builds.sr.ht](https://builds.sr.ht/~mariusor/fedbox) manifest to different files:
    * One to run unit-tests and coverage
    * Two for integration tests using the boltdb and badger storage backends
    * One to generate the container image and push it to quay.io
