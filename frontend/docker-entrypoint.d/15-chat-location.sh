#!/bin/sh
# Picks the /api/chat/ location before 20-envsubst-on-templates.sh renders nginx.conf.
set -eu

out=/etc/nginx/conf.d/chat-location.inc

# Mirrors the backend's strconv.ParseBool: only these are false.
case "${CHAT_ENABLED:-true}" in
    0 | f | F | false | FALSE | False)
        cp /etc/nginx/chat/disabled.conf "$out"
        echo "$0: chat disabled, /api/chat/ answers 503"
        ;;
    *)
        # Only these two, so nginx's own $host and friends survive.
        envsubst '${CHAT_HOST} ${CHAT_PORT}' < /etc/nginx/chat/proxy.conf > "$out"
        echo "$0: chat enabled, proxying /api/chat/ to ${CHAT_HOST}:${CHAT_PORT}"
        ;;
esac
