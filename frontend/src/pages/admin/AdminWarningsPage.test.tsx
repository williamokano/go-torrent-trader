import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { AdminWarningsPage } from "./AdminWarningsPage";

vi.mock("@/features/auth/token", () => ({
  getAccessToken: () => "test-token",
}));

vi.mock("@/config", () => ({
  getConfig: () => ({ API_URL: "http://localhost:8080" }),
}));

vi.mock("@/components/toast", () => ({
  useToast: () => ({
    success: vi.fn(),
    error: vi.fn(),
  }),
}));

const mockWarnings = [
  {
    id: 1,
    user_id: 2,
    type: "manual",
    reason: "Bad behavior",
    issued_by: 1,
    status: "active",
    lifted_at: null,
    lifted_by: null,
    lifted_reason: null,
    expires_at: null,
    created_at: "2026-03-01T00:00:00Z",
    username: "baduser",
    issued_by_name: "admin",
    lifted_by_name: null,
  },
  {
    id: 2,
    user_id: 3,
    type: "ratio_soft",
    reason: "Low ratio",
    issued_by: null,
    status: "resolved",
    lifted_at: "2026-03-05T00:00:00Z",
    lifted_by: null,
    lifted_reason: null,
    expires_at: null,
    created_at: "2026-02-20T00:00:00Z",
    username: "lowratio",
    issued_by_name: null,
    lifted_by_name: null,
  },
];

describe("AdminWarningsPage", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("renders the page with warnings table", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue({
      ok: true,
      json: async () => ({
        warnings: mockWarnings,
        total: 2,
        page: 1,
        per_page: 25,
      }),
    } as Response);

    render(
      <MemoryRouter>
        <AdminWarningsPage />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByText("baduser")).toBeInTheDocument();
    });

    expect(screen.getByText("Bad behavior")).toBeInTheDocument();
    expect(screen.getByText("Manual")).toBeInTheDocument();
    expect(screen.getByText("Lift")).toBeInTheDocument();
  });

  it("shows empty state when no warnings", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue({
      ok: true,
      json: async () => ({
        warnings: [],
        total: 0,
        page: 1,
        per_page: 25,
      }),
    } as Response);

    render(
      <MemoryRouter>
        <AdminWarningsPage />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByText("No warnings found.")).toBeInTheDocument();
    });
  });

  it("renders the Issue Warning button", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue({
      ok: true,
      json: async () => ({
        warnings: [],
        total: 0,
        page: 1,
        per_page: 25,
      }),
    } as Response);

    render(
      <MemoryRouter>
        <AdminWarningsPage />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getAllByText("Issue Warning").length).toBeGreaterThan(0);
    });
  });

  it("shows status filter and search inputs", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue({
      ok: true,
      json: async () => ({ warnings: [], total: 0 }),
    } as Response);

    render(
      <MemoryRouter>
        <AdminWarningsPage />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByLabelText("Status")).toBeInTheDocument();
      expect(screen.getByLabelText("Username")).toBeInTheDocument();
    });
  });
});
