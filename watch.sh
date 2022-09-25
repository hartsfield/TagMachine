#!/bin/bash
inotifywait -r -m -e modify . |
   while read file_path file_event file_name; do
       echo ${file_path}${file_name} event: ${file_event}
   done
