#!/bin/sh
if [ -z "$husky_skip_init" ]; then
  export husky_skip_init=1
  sh -c "$0" "$@"
  exit $?
fi
