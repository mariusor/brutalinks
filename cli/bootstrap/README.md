## Initializing the database

Your .env file should contain at least these entries:

    DB_NAME=littr
    DB_USER=littr
    DB_PASSWORD=SuperSecret,SecretPassword

Then executing the bootstrap script with the following parameters:

    cli/bootstrap -user postgres -pw postgresuserpass -host postgreshost 
