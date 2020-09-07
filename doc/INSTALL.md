# Installing

## Pre-requisites

The basic requirement for running [littr.go](https://github.com/mariusor/littr.go) locally is a 
go dev environment (version 1.11 or newer, as we require go modules support). 

```sh
$ git clone https://github.com/mariusor/littr.go
$ cd littr.go
```

### Running fed::BOX

We are now using [fedbox](https://github.com/go-ap/fedbox) as an *ActivityPub* backend.
Follow the project's [install instructions]((https://github.com/go-ap/fedbox/blob/master/doc/INSTALL.md)) to get the instance running. 

### Editing the configuration 

```sh
$ cp .env.example .env
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
$ make ENV={env} HOSTNAME={hostname} PORT={port} -C docker/ images
```
