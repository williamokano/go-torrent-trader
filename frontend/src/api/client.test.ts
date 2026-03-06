import { describe, expect, test } from "vitest";
import { api } from "@/api/client";
import type { paths } from "@/api/schema";

describe("API client", () => {
  test("api client is created and exports GET method", () => {
    expect(api).toBeDefined();
    expect(api.GET).toBeTypeOf("function");
  });

  test("api client exports standard HTTP methods", () => {
    expect(api.POST).toBeTypeOf("function");
    expect(api.PUT).toBeTypeOf("function");
    expect(api.DELETE).toBeTypeOf("function");
  });

  test("paths type includes /healthz endpoint", () => {
    // Compile-time check: this assignment will fail if paths doesn't have /healthz
    const _healthzPath: keyof paths = "/healthz";
    expect(_healthzPath).toBe("/healthz");
  });

  test("health response type has expected shape", () => {
    // Compile-time check: verify the response type structure is usable
    type HealthResponse =
      paths["/healthz"]["get"]["responses"]["200"]["content"]["application/json"];
    const mockResponse: HealthResponse = { status: "ok" };
    expect(mockResponse.status).toBe("ok");
  });
});
