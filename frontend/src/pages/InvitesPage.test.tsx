import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, test, expect, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { InvitesPage } from "@/pages/InvitesPage";

vi.mock("@/features/auth/token", () => ({
  getAccessToken: () => "fake-token",
}));

vi.mock("@/config", () => ({
  getConfig: () => ({ API_URL: "http://localhost:8080", SITE_NAME: "Test" }),
}));

vi.mock("@/features/auth", () => ({
  useAuth: () => ({
    user: {
      id: 1,
      username: "testuser",
      invites: 3,
    },
    isAuthenticated: true,
  }),
}));

const FAKE_INVITES = [
  {
    id: 1,
    token: "abc123def456ghi789jkl012mno345pq",
    status: "pending",
    expires_at: "2026-03-15T10:00:00Z",
    created_at: "2026-03-08T10:00:00Z",
  },
  {
    id: 2,
    token: "xyz789uvw456rst123opq012nml345kj",
    status: "redeemed",
    expires_at: "2026-03-10T10:00:00Z",
    created_at: "2026-03-03T10:00:00Z",
    invitee_id: 42,
    redeemed_at: "2026-03-04T10:00:00Z",
  },
];

const mockFetch = vi.fn();

afterEach(cleanup);

beforeEach(() => {
  vi.clearAllMocks();
  mockFetch.mockResolvedValue({
    ok: true,
    json: () =>
      Promise.resolve({
        invites: FAKE_INVITES,
        total: 2,
        page: 1,
        per_page: 25,
      }),
  });
  vi.stubGlobal("fetch", mockFetch);
});

function renderInvitesPage() {
  return render(
    <MemoryRouter initialEntries={["/invites"]}>
      <InvitesPage />
    </MemoryRouter>,
  );
}

describe("InvitesPage", () => {
  test("renders page title", () => {
    renderInvitesPage();
    expect(screen.getByText("Invitations")).toBeInTheDocument();
  });

  test("shows remaining invite count", () => {
    renderInvitesPage();
    expect(screen.getByText("Remaining invites: 3")).toBeInTheDocument();
  });

  test("shows loading state initially", () => {
    mockFetch.mockReturnValue(new Promise(() => {}));
    renderInvitesPage();
    expect(screen.getByText("Loading invites...")).toBeInTheDocument();
  });

  test("renders invite table after loading", async () => {
    renderInvitesPage();
    await waitFor(() => {
      expect(screen.getByText("abc123def456...")).toBeInTheDocument();
    });
    expect(screen.getByText("xyz789uvw456...")).toBeInTheDocument();
  });

  test("renders invite statuses", async () => {
    renderInvitesPage();
    await waitFor(() => {
      expect(screen.getByText("pending")).toBeInTheDocument();
    });
    expect(screen.getByText("redeemed")).toBeInTheDocument();
  });

  test("renders table headers", async () => {
    renderInvitesPage();
    await waitFor(() => {
      expect(screen.getByText("abc123def456...")).toBeInTheDocument();
    });
    expect(screen.getByText("Token")).toBeInTheDocument();
    expect(screen.getByText("Status")).toBeInTheDocument();
    expect(screen.getByText("Created")).toBeInTheDocument();
    expect(screen.getByText("Expires")).toBeInTheDocument();
  });

  test("shows generate invite button when user has invites", () => {
    renderInvitesPage();
    expect(screen.getByText("Generate Invite")).toBeInTheDocument();
  });

  test("shows empty state when no invites", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: () =>
        Promise.resolve({ invites: [], total: 0, page: 1, per_page: 25 }),
    });
    renderInvitesPage();
    await waitFor(() => {
      expect(screen.getByText("No invites created yet.")).toBeInTheDocument();
    });
  });

  test("shows error state on API failure", async () => {
    mockFetch.mockResolvedValue({
      ok: false,
      json: () => Promise.resolve({ error: { message: "Unauthorized" } }),
    });
    renderInvitesPage();
    await waitFor(() => {
      expect(screen.getByText("Unauthorized")).toBeInTheDocument();
    });
  });

  test("passes authorization header to fetch", async () => {
    renderInvitesPage();
    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining("/api/v1/invites"),
        expect.objectContaining({
          headers: { Authorization: "Bearer fake-token" },
        }),
      );
    });
  });
});
