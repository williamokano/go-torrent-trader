let accessToken: string | null = null;

const REFRESH_TOKEN_KEY = "torrenttrader-refresh-token";
const ACCESS_TOKEN_KEY = "torrenttrader-access-token";
const TOKEN_EXPIRY_KEY = "torrenttrader-token-expiry";

// Buffer before expiry to trigger refresh (5 minutes in ms)
const REFRESH_BUFFER_MS = 5 * 60 * 1000;

export function getAccessToken(): string | null {
  if (accessToken) return accessToken;
  // Restore from localStorage on page reload
  try {
    accessToken = localStorage.getItem(ACCESS_TOKEN_KEY);
  } catch {
    // localStorage may be unavailable
  }
  return accessToken;
}

export function setAccessToken(token: string | null): void {
  accessToken = token;
  try {
    if (token) {
      localStorage.setItem(ACCESS_TOKEN_KEY, token);
    } else {
      localStorage.removeItem(ACCESS_TOKEN_KEY);
    }
  } catch {
    // localStorage may be unavailable
  }
}

export function getTokenExpiry(): number | null {
  try {
    const val = localStorage.getItem(TOKEN_EXPIRY_KEY);
    return val ? Number(val) : null;
  } catch {
    return null;
  }
}

export function setTokenExpiry(expiresInSeconds: number): void {
  try {
    const expiresAt = Date.now() + expiresInSeconds * 1000;
    localStorage.setItem(TOKEN_EXPIRY_KEY, String(expiresAt));
  } catch {
    // localStorage may be unavailable
  }
}

export function isAccessTokenValid(): boolean {
  const token = getAccessToken();
  if (!token) return false;
  const expiry = getTokenExpiry();
  if (!expiry) return false;
  // Valid if not expired minus buffer
  return Date.now() < expiry - REFRESH_BUFFER_MS;
}

export function getRefreshToken(): string | null {
  try {
    return localStorage.getItem(REFRESH_TOKEN_KEY);
  } catch {
    return null;
  }
}

export function setRefreshToken(token: string | null): void {
  try {
    if (token) {
      localStorage.setItem(REFRESH_TOKEN_KEY, token);
    } else {
      localStorage.removeItem(REFRESH_TOKEN_KEY);
    }
  } catch {
    // localStorage may be unavailable
  }
}

export function clearTokens(): void {
  accessToken = null;
  try {
    localStorage.removeItem(REFRESH_TOKEN_KEY);
    localStorage.removeItem(ACCESS_TOKEN_KEY);
    localStorage.removeItem(TOKEN_EXPIRY_KEY);
  } catch {
    // localStorage may be unavailable
  }
}
