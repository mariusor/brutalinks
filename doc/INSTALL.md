# Installing

## Pre-requisites

The basic requirement for running [BrutaLinks](https://git.sr./~mariusor/brutalinks) locally is a
go dev environment (version 1.18 or newer).

```sh
$ git clone https://git.sr.ht/~mariusor/brutalinks
$ cd brutalinks
```

### Running Fed::BOX

We are now using [fedbox](https://github.com/go-ap/fedbox) as an *ActivityPub* backend.
Follow the project's [install instructions]((https://github.com/go-ap/fedbox/blob/master/doc/INSTALL.md)) to get the instance running. We'll assume your instance is https://fedbox.example.com

After FedBOX is running, you need to create the required brutalinks actors:

```sh
# This creates an OAuth2 account and ActivityPub Application Actor for Brutalinks.
$ fedboxctl oauth client add --redirectUri https://brutalinks.example.com/callback
client's pw:
pw again:
Client ID: 4f449c81-1dbb-dead-beef-5a83926a0fbf

# The Actor can be found at: https://fedbox.example.com/actors/4f449c81-1dbb-dead-beef-5a83926a0fbf

# We can now create some of the additional objects:

# First we create tags for the instance operators and moderators:
$ fedboxctl ap add --name "#sysop" --name "#mod" \
--attributedTo https://fedbox.example.com/actors/4f449c81-1dbb-dead-beef-5a83926a0fbf

# This creates a Person actor named "admin" with the #sysop tag
$ fedboxctl ap actor add admin --tag "#sysop"
admin's pw:
pw again:
Added "Person" [admin]: https://fedbox.example.com/actors/310a1a7c-dead-beef-d00d-9a6a8e40acdf

```

### Editing the configuration

```sh
$ cp .env.dist .env
$ $EDITOR .env
```

You need to set `API_URL` environment variable to the URL at which Fed::BOX can be reached at: `https://fedbox.example.com`

You need to set the `OAUTH2_KEY` to the Client ID: `4f449c81-1dbb-dead-beef-5a83926a0fbf` and the `OAUTH2_SECRET` to the password you supplied when adding the client.


## Running

Running the application in development mode is as simple as:

```sh
$ make run
```
