#!/bin/bash
export testPass=
export hmacss=
export redisIP=
trap -- '' SIGTERM
git pull
go build -o TagMachine
pkill -f TagMachine
nohup ./TagMachine > /dev/null & disown
sleep 2