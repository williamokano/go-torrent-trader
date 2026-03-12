import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, test, expect, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { HomePage } from "@/pages/HomePage";

const mockGET = vi.fn();

vi.mock("@/api", () => ({
  api: {
    GET: (...args: unknown[]) => mockGET(...args),
  },
}));

vi.mock("@/features/auth", () => ({
  useAuth: () => ({
    user: null,
    isAuthenticated: false,
    isLoading: false,
    login: vi.fn(),
    logout: vi.fn(),
    register: vi.fn(),
  }),
}));

vi.mock("@/features/auth/token", () => ({
  getAccessToken: () => "fake-token",
}));

const FAKE_STATS = {
  users: 100,
  torrents: 500,
  peers: 73,
  online_users: 5,
};

const FAKE_TORRENTS = [
  {
    id: 1,
    name: "Ubuntu 24.04 LTS Desktop",
    info_hash: "abc123",
    size: 4_800_000_000,
    category_id: 1,
    uploader_id: 1,
    anonymous: false,
    uploader_name: "testuser1",
    seeders: 42,
    leechers: 5,
    times_completed: 318,
    comments_count: 0,
    file_count: 1,
    created_at: "2026-03-05T14:30:00Z",
    updated_at: "2026-03-05T14:30:00Z",
  },
];

afterEach(cleanup);

beforeEach(() => {
  vi.clearAllMocks();
  mockGET.mockImplementation((url: string) => {
    if (url === "/api/v1/stats") {
      return Promise.resolve({
        data: { stats: FAKE_STATS },
        error: undefined,
      });
    }
    if (url === "/api/v1/torrents") {
      return Promise.resolve({
        data: { torrents: FAKE_TORRENTS, total: 1, page: 1, per_page: 5 },
        error: undefined,
      });
    }
    return Promise.resolve({ data: undefined, error: undefined });
  });
});

function renderHomePage() {
  return render(
    <MemoryRouter>
      <HomePage />
    </MemoryRouter>,
  );
}

describe("HomePage", () => {
  test("renders welcome message", () => {
    renderHomePage();
    expect(screen.getByText("Welcome to TorrentTrader")).toBeInTheDocument();
  });

  test("renders description for unauthenticated users", () => {
    renderHomePage();
    expect(
      screen.getByText("Your private BitTorrent tracker community."),
    ).toBeInTheDocument();
  });

  test("renders stats section with real data", async () => {
    renderHomePage();
    await waitFor(() => {
      expect(screen.getByLabelText("Site statistics")).toBeInTheDocument();
      expect(screen.getByText("Users")).toBeInTheDocument();
      expect(screen.getByText("Torrents")).toBeInTheDocument();
      expect(screen.getByText("Peers")).toBeInTheDocument();
      expect(screen.getByText("100")).toBeInTheDocument();
      expect(screen.getByText("500")).toBeInTheDocument();
      expect(screen.getByText("73")).toBeInTheDocument();
    });
  });

  test("shows loading state for stats", () => {
    mockGET.mockImplementation((url: string) => {
      if (url === "/api/v1/stats") {
        return new Promise(() => {}); // never resolves
      }
      return Promise.resolve({
        data: { torrents: [], total: 0, page: 1, per_page: 5 },
        error: undefined,
      });
    });
    renderHomePage();
    expect(screen.getByText("Loading stats...")).toBeInTheDocument();
  });

  test("hides stats section on API failure", async () => {
    mockGET.mockImplementation((url: string) => {
      if (url === "/api/v1/stats") {
        return Promise.resolve({
          data: undefined,
          error: { error: { message: "DB down" } },
        });
      }
      return Promise.resolve({
        data: { torrents: FAKE_TORRENTS, total: 1, page: 1, per_page: 5 },
        error: undefined,
      });
    });
    renderHomePage();
    await waitFor(() => {
      // Stats labels should not be rendered when stats is null
      expect(screen.queryByText("Loading stats...")).not.toBeInTheDocument();
    });
    // The section element exists but has no stat cards
    expect(screen.queryByText("Users")).not.toBeInTheDocument();
  });

  test("fetches stats from /api/v1/stats", async () => {
    renderHomePage();
    await waitFor(() => {
      expect(mockGET).toHaveBeenCalledWith("/api/v1/stats");
    });
  });

  test("renders latest torrents section title", () => {
    renderHomePage();
    expect(screen.getByText("Latest Torrents")).toBeInTheDocument();
  });

  test("shows loading state initially for torrents", () => {
    mockGET.mockImplementation((url: string) => {
      if (url === "/api/v1/stats") {
        return Promise.resolve({
          data: { stats: FAKE_STATS },
          error: undefined,
        });
      }
      return new Promise(() => {}); // never resolves
    });
    renderHomePage();
    expect(screen.getByText("Loading...")).toBeInTheDocument();
  });

  test("renders latest torrents after loading", async () => {
    renderHomePage();
    await waitFor(() => {
      expect(screen.getByText("Ubuntu 24.04 LTS Desktop")).toBeInTheDocument();
    });
  });

  test("shows empty state when no torrents", async () => {
    mockGET.mockImplementation((url: string) => {
      if (url === "/api/v1/stats") {
        return Promise.resolve({
          data: { stats: FAKE_STATS },
          error: undefined,
        });
      }
      return Promise.resolve({
        data: { torrents: [], total: 0, page: 1, per_page: 5 },
        error: undefined,
      });
    });
    renderHomePage();
    await waitFor(() => {
      expect(screen.getByText("No torrents yet.")).toBeInTheDocument();
    });
  });

  test("shows error state on torrents API failure", async () => {
    mockGET.mockImplementation((url: string) => {
      if (url === "/api/v1/stats") {
        return Promise.resolve({
          data: { stats: FAKE_STATS },
          error: undefined,
        });
      }
      return Promise.resolve({
        data: undefined,
        error: { error: { message: "Network error" } },
      });
    });
    renderHomePage();
    await waitFor(() => {
      expect(screen.getByText("Network error")).toBeInTheDocument();
    });
  });

  test("fetches latest torrents sorted by created_at desc", async () => {
    renderHomePage();
    await waitFor(() => {
      expect(mockGET).toHaveBeenCalledWith(
        "/api/v1/torrents",
        expect.objectContaining({
          params: expect.objectContaining({
            query: expect.objectContaining({
              per_page: 5,
              sort: "created_at",
              order: "desc",
            }),
          }),
        }),
      );
    });
  });

  test("passes authorization header for torrents", async () => {
    renderHomePage();
    await waitFor(() => {
      expect(mockGET).toHaveBeenCalledWith(
        "/api/v1/torrents",
        expect.objectContaining({
          headers: { Authorization: "Bearer fake-token" },
        }),
      );
    });
  });
});
