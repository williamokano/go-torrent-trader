// Dev config — overridden by docker-entrypoint.sh in production.
// This file is served from /public during vite dev and copied to dist on build.
window.__CONFIG__ = {
  API_URL: "http://localhost:8080",
  SITE_NAME: "TorrentTrader",
};
