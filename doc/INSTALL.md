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

Running with docker is no longer fully supported, since the move to using fedbox. See #26. 

<!--
Go to the littr.go working directory and copy your `.env` file to the docker folder:

```sh
$ cp .env ./docker/
```

In the `docker/.env` file we need to modify the `DB_HOST` value to match the name of the postgres container from the 
[docker/docker-compose.yaml](../docker/docker-compose.yaml). The default is `db`.

Then, as a user in the docker group, just run:
```sh
$ make compose
```
-->
