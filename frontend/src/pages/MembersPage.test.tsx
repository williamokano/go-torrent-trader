import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, test, expect, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { MembersPage } from "@/pages/MembersPage";

vi.mock("@/features/auth/token", () => ({
  getAccessToken: () => "fake-token",
}));

vi.mock("@/config", () => ({
  getConfig: () => ({ API_URL: "http://localhost:8080", SITE_NAME: "Test" }),
}));

const FAKE_MEMBERS = [
  {
    id: 1,
    username: "alice",
    group_id: 5,
    group_name: "User",
    uploaded: 1073741824,
    downloaded: 536870912,
    ratio: 2.0,
    donor: false,
    created_at: "2025-06-01T10:00:00Z",
  },
  {
    id: 2,
    username: "bob",
    group_id: 1,
    group_name: "Administrator",
    uploaded: 0,
    downloaded: 0,
    ratio: 0,
    donor: true,
    created_at: "2025-01-15T08:00:00Z",
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
        users: FAKE_MEMBERS,
        total: 2,
        page: 1,
        per_page: 25,
      }),
  });
  vi.stubGlobal("fetch", mockFetch);
});

function renderMembersPage(initialEntries = ["/members"]) {
  return render(
    <MemoryRouter initialEntries={initialEntries}>
      <MembersPage />
    </MemoryRouter>,
  );
}

describe("MembersPage", () => {
  test("renders page title", () => {
    renderMembersPage();
    expect(screen.getByText("Members")).toBeInTheDocument();
  });

  test("shows loading state initially", () => {
    mockFetch.mockReturnValue(new Promise(() => {}));
    renderMembersPage();
    expect(screen.getByText("Loading members...")).toBeInTheDocument();
  });

  test("renders member table after loading", async () => {
    renderMembersPage();
    await waitFor(() => {
      expect(screen.getByText("alice")).toBeInTheDocument();
    });
    expect(screen.getByText("bob")).toBeInTheDocument();
  });

  test("renders group names", async () => {
    renderMembersPage();
    await waitFor(() => {
      expect(screen.getByText("User")).toBeInTheDocument();
    });
    expect(screen.getByText("Administrator")).toBeInTheDocument();
  });

  test("renders donor badge", async () => {
    renderMembersPage();
    await waitFor(() => {
      expect(screen.getByText("Donor")).toBeInTheDocument();
    });
  });

  test("renders upload/download stats", async () => {
    renderMembersPage();
    await waitFor(() => {
      expect(screen.getByText("1.00 GB")).toBeInTheDocument();
    });
    expect(screen.getByText("512.00 MB")).toBeInTheDocument();
  });

  test("renders ratio", async () => {
    renderMembersPage();
    await waitFor(() => {
      expect(screen.getByText("2.00")).toBeInTheDocument();
    });
  });

  test("renders search input", () => {
    renderMembersPage();
    expect(
      screen.getByPlaceholderText("Search members..."),
    ).toBeInTheDocument();
  });

  test("shows empty state when no members", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: () =>
        Promise.resolve({ users: [], total: 0, page: 1, per_page: 25 }),
    });
    renderMembersPage();
    await waitFor(() => {
      expect(screen.getByText("No members found.")).toBeInTheDocument();
    });
  });

  test("shows error state on API failure", async () => {
    mockFetch.mockResolvedValue({
      ok: false,
      json: () => Promise.resolve({ error: { message: "Unauthorized" } }),
    });
    renderMembersPage();
    await waitFor(() => {
      expect(screen.getByText("Unauthorized")).toBeInTheDocument();
    });
  });

  test("passes authorization header to fetch", async () => {
    renderMembersPage();
    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining("/api/v1/users"),
        expect.objectContaining({
          headers: { Authorization: "Bearer fake-token" },
        }),
      );
    });
  });

  test("renders table headers", async () => {
    renderMembersPage();
    await waitFor(() => {
      expect(screen.getByText("alice")).toBeInTheDocument();
    });
    expect(screen.getByText("Username")).toBeInTheDocument();
    expect(screen.getByText("Group")).toBeInTheDocument();
    expect(screen.getByText("Uploaded")).toBeInTheDocument();
    expect(screen.getByText("Downloaded")).toBeInTheDocument();
    expect(screen.getByText("Ratio")).toBeInTheDocument();
    expect(screen.getByText("Joined")).toBeInTheDocument();
  });
});
