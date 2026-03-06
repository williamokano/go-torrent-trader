import { beforeEach, describe, expect, test, vi } from "vitest";
import {
  clearTokens,
  getAccessToken,
  getRefreshToken,
  isAccessTokenValid,
  setAccessToken,
  setRefreshToken,
  setTokenExpiry,
} from "@/features/auth/token";

describe("token management", () => {
  beforeEach(() => {
    clearTokens();
    localStorage.clear();
  });

  describe("access token", () => {
    test("returns null initially", () => {
      expect(getAccessToken()).toBeNull();
    });

    test("roundtrip set and get", () => {
      setAccessToken("test-access-token");
      expect(getAccessToken()).toBe("test-access-token");
    });

    test("persists to localStorage", () => {
      setAccessToken("persisted-token");
      expect(localStorage.getItem("torrenttrader-access-token")).toBe(
        "persisted-token",
      );
    });

    test("set to null clears it", () => {
      setAccessToken("test-access-token");
      setAccessToken(null);
      expect(getAccessToken()).toBeNull();
    });
  });

  describe("refresh token", () => {
    test("returns null when no token stored", () => {
      expect(getRefreshToken()).toBeNull();
    });

    test("roundtrip set and get via localStorage", () => {
      setRefreshToken("test-refresh-token");
      expect(getRefreshToken()).toBe("test-refresh-token");
      expect(localStorage.getItem("torrenttrader-refresh-token")).toBe(
        "test-refresh-token",
      );
    });

    test("set to null removes from localStorage", () => {
      setRefreshToken("test-refresh-token");
      setRefreshToken(null);
      expect(getRefreshToken()).toBeNull();
      expect(localStorage.getItem("torrenttrader-refresh-token")).toBeNull();
    });
  });

  describe("token expiry", () => {
    test("isAccessTokenValid returns false without token", () => {
      expect(isAccessTokenValid()).toBe(false);
    });

    test("isAccessTokenValid returns false without expiry", () => {
      setAccessToken("token");
      expect(isAccessTokenValid()).toBe(false);
    });

    test("isAccessTokenValid returns true when not expired", () => {
      setAccessToken("token");
      setTokenExpiry(3600); // 1 hour from now
      expect(isAccessTokenValid()).toBe(true);
    });

    test("isAccessTokenValid returns false when expired", () => {
      setAccessToken("token");
      // Set expiry in the past
      vi.spyOn(Date, "now").mockReturnValue(1000);
      setTokenExpiry(1);
      vi.spyOn(Date, "now").mockReturnValue(1000 + 2 * 1000); // 2 seconds later, past expiry
      expect(isAccessTokenValid()).toBe(false);
      vi.restoreAllMocks();
    });

    test("isAccessTokenValid returns false within refresh buffer", () => {
      setAccessToken("token");
      setTokenExpiry(4 * 60); // 4 minutes — within 5 min buffer
      expect(isAccessTokenValid()).toBe(false);
    });
  });

  describe("clearTokens", () => {
    test("clears all tokens and expiry", () => {
      setAccessToken("access");
      setRefreshToken("refresh");
      setTokenExpiry(3600);

      clearTokens();

      expect(getAccessToken()).toBeNull();
      expect(getRefreshToken()).toBeNull();
      expect(localStorage.getItem("torrenttrader-refresh-token")).toBeNull();
      expect(localStorage.getItem("torrenttrader-access-token")).toBeNull();
      expect(localStorage.getItem("torrenttrader-token-expiry")).toBeNull();
    });
  });
});
