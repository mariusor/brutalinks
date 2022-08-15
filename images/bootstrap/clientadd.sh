#!/usr/bin/expect -f
# Special thanks to github.com/squash for providing this script in the course of solving
# https://github.com/mariusor/go-littr/issues/38#issuecomment-658800183

set pass [lindex $argv 0]
set callback_url [lindex $argv 1]

spawn ctl oauth client add --redirectUri "${callback_url}"
expect "client's pw: "
send "${pass}\r"
expect "pw again: "
send "${pass}\r"
expect eof

