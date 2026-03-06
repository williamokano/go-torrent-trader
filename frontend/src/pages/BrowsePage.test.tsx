import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { userEvent } from "@testing-library/user-event";
import { afterEach, beforeEach, describe, test, expect, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { BrowsePage } from "@/pages/BrowsePage";

const mockGET = vi.fn();

vi.mock("@/api", () => ({
  api: {
    GET: (...args: unknown[]) => mockGET(...args),
  },
}));

vi.mock("@/features/auth/token", () => ({
  getAccessToken: () => "fake-token",
}));

const FAKE_CATEGORIES = [
  { id: 1, name: "Movies", parent_id: null, sort_order: 1 },
  { id: 2, name: "TV", parent_id: null, sort_order: 2 },
];

const FAKE_TORRENTS = [
  {
    id: 1,
    name: "Ubuntu 24.04 LTS Desktop",
    info_hash: "abc123",
    size: 4_800_000_000,
    category_id: 1,
    category_name: "Movies",
    uploader_id: 1,
    anonymous: false,
    seeders: 42,
    leechers: 5,
    times_completed: 318,
    comments_count: 0,
    file_count: 1,
    created_at: "2026-03-05T14:30:00Z",
    updated_at: "2026-03-05T14:30:00Z",
  },
  {
    id: 2,
    name: "Arch Linux 2026.03.01",
    info_hash: "def456",
    size: 850_000_000,
    category_id: 1,
    category_name: "Movies",
    uploader_id: 2,
    anonymous: false,
    seeders: 28,
    leechers: 3,
    times_completed: 156,
    comments_count: 0,
    file_count: 1,
    created_at: "2026-03-04T10:15:00Z",
    updated_at: "2026-03-04T10:15:00Z",
  },
];

afterEach(cleanup);

beforeEach(() => {
  vi.clearAllMocks();
  mockGET.mockImplementation((url: string) => {
    if (url === "/api/v1/categories") {
      return Promise.resolve({
        data: { categories: FAKE_CATEGORIES },
        error: undefined,
      });
    }
    return Promise.resolve({
      data: { torrents: FAKE_TORRENTS, total: 2, page: 1, per_page: 5 },
      error: undefined,
    });
  });
});

function renderBrowsePage(initialEntries = ["/browse"]) {
  return render(
    <MemoryRouter initialEntries={initialEntries}>
      <BrowsePage />
    </MemoryRouter>,
  );
}

describe("BrowsePage", () => {
  test("renders page title", () => {
    renderBrowsePage();
    expect(screen.getByText("Browse Torrents")).toBeInTheDocument();
  });

  test("shows loading state initially", () => {
    mockGET.mockImplementation((url: string) => {
      if (url === "/api/v1/categories") {
        return Promise.resolve({
          data: { categories: FAKE_CATEGORIES },
          error: undefined,
        });
      }
      return new Promise(() => {}); // never resolves for torrents
    });
    renderBrowsePage();
    expect(screen.getByText("Loading torrents...")).toBeInTheDocument();
  });

  test("renders torrent table after loading", async () => {
    renderBrowsePage();
    await waitFor(() => {
      expect(screen.getByText("Ubuntu 24.04 LTS Desktop")).toBeInTheDocument();
    });
    expect(screen.getByText("Arch Linux 2026.03.01")).toBeInTheDocument();
  });

  test("renders search input", () => {
    renderBrowsePage();
    expect(
      screen.getByPlaceholderText("Search torrents..."),
    ).toBeInTheDocument();
  });

  test("renders category filter", () => {
    renderBrowsePage();
    expect(screen.getByLabelText("Category")).toBeInTheDocument();
  });

  test("renders sort select", () => {
    renderBrowsePage();
    expect(screen.getByLabelText("Sort by")).toBeInTheDocument();
  });

  test("shows empty state when no torrents", async () => {
    mockGET.mockImplementation((url: string) => {
      if (url === "/api/v1/categories") {
        return Promise.resolve({
          data: { categories: FAKE_CATEGORIES },
          error: undefined,
        });
      }
      return Promise.resolve({
        data: { torrents: [], total: 0, page: 1, per_page: 5 },
        error: undefined,
      });
    });
    renderBrowsePage();
    await waitFor(() => {
      expect(screen.getByText("No torrents found.")).toBeInTheDocument();
    });
  });

  test("shows error state on API failure", async () => {
    mockGET.mockImplementation((url: string) => {
      if (url === "/api/v1/categories") {
        return Promise.resolve({
          data: { categories: FAKE_CATEGORIES },
          error: undefined,
        });
      }
      return Promise.resolve({
        data: undefined,
        error: { error: { message: "Server error" } },
      });
    });
    renderBrowsePage();
    await waitFor(() => {
      expect(screen.getByText("Server error")).toBeInTheDocument();
    });
  });

  test("passes search query to API", async () => {
    renderBrowsePage(["/browse?q=ubuntu"]);
    await waitFor(() => {
      expect(mockGET).toHaveBeenCalledWith(
        "/api/v1/torrents",
        expect.objectContaining({
          params: expect.objectContaining({
            query: expect.objectContaining({ search: "ubuntu" }),
          }),
        }),
      );
    });
  });

  test("passes category filter to API", async () => {
    renderBrowsePage(["/browse?cat=2"]);
    await waitFor(() => {
      expect(mockGET).toHaveBeenCalledWith(
        "/api/v1/torrents",
        expect.objectContaining({
          params: expect.objectContaining({
            query: expect.objectContaining({ cat: 2 }),
          }),
        }),
      );
    });
  });

  test("renders health indicators after loading", async () => {
    renderBrowsePage();
    await waitFor(() => {
      expect(screen.getByText("Ubuntu 24.04 LTS Desktop")).toBeInTheDocument();
    });
    const healthDots = document.querySelectorAll(".browse__health");
    expect(healthDots.length).toBeGreaterThan(0);
  });

  test("passes authorization header", async () => {
    renderBrowsePage();
    await waitFor(() => {
      expect(mockGET).toHaveBeenCalledWith(
        "/api/v1/torrents",
        expect.objectContaining({
          headers: { Authorization: "Bearer fake-token" },
        }),
      );
    });
  });

  test("search input triggers re-fetch with new query", async () => {
    renderBrowsePage();
    const user = userEvent.setup();
    await waitFor(() => {
      expect(screen.getByText("Ubuntu 24.04 LTS Desktop")).toBeInTheDocument();
    });

    const searchInput = screen.getByPlaceholderText("Search torrents...");
    await user.type(searchInput, "a");

    await waitFor(() => {
      expect(mockGET).toHaveBeenLastCalledWith(
        "/api/v1/torrents",
        expect.objectContaining({
          params: expect.objectContaining({
            query: expect.objectContaining({ search: "a" }),
          }),
        }),
      );
    });
  });
});
