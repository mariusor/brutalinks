# Installing

## Pre-requisites

The basic requirements for running [littr.go](https://github.com/mariusor/littr.go) locally are a PostgreSQL server 
(version 9.5 or newer) with the `ltree` module, and a go dev environment 
(version 1.11 or newer, as we require go modules support). 

    $ git clone https://github.com/mariusor/littr.go
    $ cd littr.go
    $ cp .env.example .env
    $ $EDITOR .env

## Bootstrapping the database

The database is created using the `bootstrap` binary which uses the configuration settings in the `.env` 
file created previously. 

The example below assumes a default PostgreSQL installation which allows the admin user 
to log-in without a password when connecting from localhost.

    $ grep DB_ .env
    DB_HOST=localhost
    DB_NAME=littr-dev
    DB_USER=littr-dev
    DB_PASSWORD=super-secret-secret-password
    POSTGRESS_PASSWORD=RootPW-also-secret

    $ make bootstrap 
    $ ./run.sh bin/bootstrap -user postgres

## Running 

Running the application in development mode is as simple as: 

    $ make run

# Docker

Running with docker is really easy. 

Go to the littr.go working directory and copy your `.env` file to the docker folder:

    $ cp .env ./docker/

In the `docker/.env` file we need to modify the `DB_HOST` value to match the name of the postgres container from the 
[docker/docker-compose.yaml](../docker/docker-compose.yaml). The default is `db`.

Then, as a user in the docker group, just run:

    $ make compose
