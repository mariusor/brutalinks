## Running score updates

Your .env file should contain at least these entries:

    DB_NAME=littr
    DB_USER=littr
    DB_PASSWORD=SuperSecret,SecretPassword

You can execute the score update script by calling it with the following parameters:

    cli/votes -since 2h # loads all items from past two hours and updates the scores
    
    cli/votes -key {hash} # loads specific item and updates the score
    
This binary is meant to be invoked periodically using a cron or a systemd timer.
