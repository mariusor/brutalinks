# Installing

## Pre-requisites

The basic requirement for running [go-littr](https://github.com/mariusor/go-littr) locally is a 
go dev environment (version 1.11 or newer, as we require go modules support). 

```sh
$ git clone https://github.com/mariusor/go-littr
$ cd go-littr
```

### Running fed::BOX

We are now using [fedbox](https://github.com/go-ap/fedbox) as an *ActivityPub* backend.
Follow the project's [install instructions]((https://github.com/go-ap/fedbox/blob/master/doc/INSTALL.md)) to get the instance running. 

### Editing the configuration 

```sh
$ cp .env.dist .env
$ $EDITOR .env
```

You need to set `API_URL` environment variable to the fedbox url from the previous step.

## Running 

Running the application in development mode is as simple as: 

```sh
$ make run
```

# Docker

As a user in the docker group, just run:

```sh 
$ make \
    HOSTNAME={hostname} \
    FEDBOX_HOSTNAME={fedbox_hostname} \
    OAUTH2_SECRET={oauth_client_pass} \
    ADMIN_PW={admin_pass} \ # optional
    -C docker/ images
```

The {hostname} and {fedbox_hostname} are the hosts that the loadbalancer listens for on port 8443.

The {oauth_client_pass} is the password that we set-up for the littr application in fedbox.

The {admin_pass} password can be missing and there's no default admin user created.
