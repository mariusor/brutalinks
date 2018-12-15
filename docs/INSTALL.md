# Installing

## Pre-requisites

The basic requirements for running littr.go locally are a postgresql server (version 9.5 or newer)
and a go environment (version 1.11 or newer). 

    $ go get github.com/mariusor/littr.go
    $ cp .env.example .env
    $ $EDITOR .env

In the `.env` file you can set-up the connection to the postgresql database and various other knobs and switches
to configure the application.

## Running 

    $ make run
