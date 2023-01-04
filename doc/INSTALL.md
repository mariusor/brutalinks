# Installing

## Pre-requisites

The basic requirement for running [go-littr](https://github.com/mariusor/go-littr) locally is a
go dev environment (version 1.11 or newer, as we require go modules support).

```sh
$ git clone https://git.sr.ht/~mariusor/brutalinks
$ cd brutalinks
```

### Running Fed::BOX

We are now using [fedbox](https://github.com/go-ap/fedbox) as an *ActivityPub* backend.
Follow the project's [install instructions]((https://github.com/go-ap/fedbox/blob/master/doc/INSTALL.md)) to get the instance running.

After Fed::BOX is running, you need to create the required brutalinks actors:

```sh
# This creates an OAuth2 account and ActivityPub Application actor for Brutalinks.
$ fedboxctl oauth client add --redirectUri https://brutalinks.example.com/callback
client's pw:
pw again:

# This creates a Person actor with an admin tag
$ fedboxctl ap actor add admin -tags #sysop
admin's pw:
pw again:

```

### Editing the configuration

```sh
$ cp .env.dist .env
$ $EDITOR .env
```

You need to set `API_URL` environment variable to the URL at which FedBOX can be reached at.

## Running

Running the application in development mode is as simple as:

```sh
$ make run
```
