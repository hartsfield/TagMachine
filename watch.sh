#!/bin/sh
# while inotifywait -r -e modify ../reddipost; do
#   go build
#   ./redipost
# done
while true; do
  go build
  $@ &
  PID=$!
  inotifywait -r -e modify .
  kill $PID
done
