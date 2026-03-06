/**
 * Runtime configuration loaded from /config.js (injected by Docker entrypoint)
 * or falling back to Vite env vars for local development.
 */

interface RuntimeConfig {
  API_URL: string;
  SITE_NAME: string;
}

declare global {
  interface Window {
    __CONFIG__?: RuntimeConfig;
  }
}

export function getConfig(): RuntimeConfig {
  // Docker runtime config takes precedence (set by docker-entrypoint.sh)
  if (window.__CONFIG__) {
    return window.__CONFIG__;
  }

  // Fallback to Vite build-time env vars (local development)
  return {
    API_URL: import.meta.env.VITE_API_URL || "http://localhost:8080",
    SITE_NAME: import.meta.env.VITE_SITE_NAME || "TorrentTrader",
  };
}
