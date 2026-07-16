import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, test, expect, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { CompletedPage } from "@/pages/CompletedPage";

vi.mock("@/features/auth/token", () => ({
  getAccessToken: () => "fake-token",
}));

vi.mock("@/config", () => ({
  getConfig: () => ({ API_URL: "http://localhost:8080", SITE_NAME: "Test" }),
}));

vi.mock("@/features/auth", () => ({
  useAuth: () => ({
    user: { id: 42, username: "testuser" },
    isAuthenticated: true,
    isLoading: false,
    login: vi.fn(),
    logout: vi.fn(),
    register: vi.fn(),
    refreshUser: vi.fn(),
  }),
}));

const mockFetch = vi.fn();

afterEach(cleanup);

beforeEach(() => {
  vi.clearAllMocks();
  vi.stubGlobal("fetch", mockFetch);
});

function renderPage() {
  return render(
    <MemoryRouter>
      <CompletedPage />
    </MemoryRouter>,
  );
}

describe("CompletedPage", () => {
  test("shows loading state initially", () => {
    mockFetch.mockReturnValue(new Promise(() => {}));
    renderPage();
    expect(screen.getByText("Loading...")).toBeInTheDocument();
  });

  test("renders history items from the activity endpoint", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: () =>
        Promise.resolve({
          activity: [
            {
              torrent_id: 5,
              torrent_name: "Some.Linux.ISO",
              uploaded: 1073741824,
              downloaded: 536870912,
              ratio: 2.0,
              seeder: false,
              completed_at: "2025-06-01T10:00:00Z",
              last_announce: "2025-06-02T10:00:00Z",
            },
          ],
          total: 1,
        }),
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Some.Linux.ISO")).toBeInTheDocument();
    });
    expect(screen.getByText("1.00 GB")).toBeInTheDocument();
    expect(screen.getByText("512.00 MB")).toBeInTheDocument();
    expect(screen.getByText("2.00")).toBeInTheDocument();
  });

  test("fetches the current user's history from the correct URL", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ activity: [], total: 0 }),
    });

    renderPage();

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "http://localhost:8080/api/v1/users/42/activity?tab=history&page=1&per_page=25",
        expect.objectContaining({
          headers: { Authorization: "Bearer fake-token" },
        }),
      );
    });
  });

  test("shows empty state when there is no history", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ activity: [], total: 0 }),
    });

    renderPage();

    await waitFor(() => {
      expect(
        screen.getByText("You haven't completed any downloads yet."),
      ).toBeInTheDocument();
    });
  });

  test("shows an error message when the request fails", async () => {
    mockFetch.mockResolvedValue({
      ok: false,
      json: () => Promise.resolve({ error: { message: "nope" } }),
    });

    renderPage();

    await waitFor(() => {
      expect(
        screen.getByText("Failed to load completed downloads"),
      ).toBeInTheDocument();
    });
  });

  test("clicking Next refetches page 2", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: () =>
        Promise.resolve({
          activity: [
            {
              torrent_id: 5,
              torrent_name: "Some.Linux.ISO",
              uploaded: 1073741824,
              downloaded: 536870912,
              ratio: 2.0,
              seeder: false,
              completed_at: "2025-06-01T10:00:00Z",
            },
          ],
          total: 30, // 2 pages at per_page=25
        }),
    });

    const user = userEvent.setup();
    renderPage();

    await screen.findByText("Some.Linux.ISO");
    await user.click(screen.getByRole("button", { name: "Next" }));

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "http://localhost:8080/api/v1/users/42/activity?tab=history&page=2&per_page=25",
        expect.objectContaining({
          headers: { Authorization: "Bearer fake-token" },
        }),
      );
    });
  });
});
