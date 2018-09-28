## Generator for default user ssh-keys

Your .env file should contain at least these entries:

    DB_NAME=littr
    DB_USER=littr
    DB_PASSWORD=SuperSecret,SecretPassword

You can execute the score update script by calling it with the following parameters:

    cli/keys -seed 6652 # generates keys for all accounts missing them using 6652 as seed for crypto functions

    cli/keys -seed 6652 -handle johndoe # generates keys for account with johndoe handle using 6652 seed 

This binary is meant to be invoked periodically using a cron or a systemd timer.
