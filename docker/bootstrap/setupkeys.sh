#!/usr/bin/expect -f
# Special thanks to github.com/squash for providing this script in the course of solving
# https://github.com/mariusor/go-littr/issues/38#issuecomment-658800183

set oauthpass [lindex $argv 0]
set hostname [lindex $argv 1]
set adminpass [lindex $argv 2]

spawn ctl ap actor add admin
expect "admin's pw: "
send "$adminpass\r"
expect "pw again: "
send "$adminpass\r"
expect eof

spawn ctl oauth client add --redirectUri http://${hostname}/auth/fedbox/callback
expect "client's pw: "
send "$oauthpass\r"
expect "pw again: "
send "$oauthpass\r"
expect eof

