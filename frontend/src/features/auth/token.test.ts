import { beforeEach, describe, expect, test } from "vitest";
import {
  clearTokens,
  getAccessToken,
  getRefreshToken,
  setAccessToken,
  setRefreshToken,
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

  describe("clearTokens", () => {
    test("clears both access and refresh tokens", () => {
      setAccessToken("access");
      setRefreshToken("refresh");

      clearTokens();

      expect(getAccessToken()).toBeNull();
      expect(getRefreshToken()).toBeNull();
      expect(localStorage.getItem("torrenttrader-refresh-token")).toBeNull();
    });
  });
});
