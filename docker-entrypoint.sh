#!/bin/sh
# Drop to the dcalcon user after ensuring /data is writable (named volumes
# from older images are often still root-owned).
set -e
mkdir -p /data
if [ "$(id -u)" = 0 ]; then
	chown -R dcalcon:dcalcon /data || true
	if command -v setpriv >/dev/null 2>&1; then
		exec setpriv --reuid=dcalcon --regid=dcalcon --init-groups -- /dcalcon "$@"
	fi
fi
exec /dcalcon "$@"
