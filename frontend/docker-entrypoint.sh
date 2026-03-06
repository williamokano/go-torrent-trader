#!/bin/sh
# Generate runtime config from environment variables.
# This runs on container startup, before nginx serves the app.

cat > /usr/share/nginx/html/config.js <<EOF
window.__CONFIG__ = {
  API_URL: "${API_URL:-http://localhost:8080}",
  SITE_NAME: "${SITE_NAME:-TorrentTrader}",
};
EOF

exec "$@"
