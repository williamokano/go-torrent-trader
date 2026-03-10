import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { AdminTorrentsPage } from "@/pages/admin/AdminTorrentsPage";
import { ToastProvider } from "@/components/toast";

const mockFetch = vi.fn();

vi.stubGlobal("fetch", mockFetch);

afterEach(cleanup);

beforeEach(() => {
  vi.clearAllMocks();
});

function renderPage() {
  return render(
    <ToastProvider>
      <MemoryRouter initialEntries={["/admin/torrents"]}>
        <AdminTorrentsPage />
      </MemoryRouter>
    </ToastProvider>,
  );
}

describe("AdminTorrentsPage", () => {
  test("renders page title", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => ({ torrents: [], total: 0 }),
    });

    renderPage();

    expect(screen.getByText("Torrents")).toBeInTheDocument();
  });

  test("shows empty state when no torrents exist", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => ({ torrents: [], total: 0 }),
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("No torrents found.")).toBeInTheDocument();
    });
  });

  test("displays torrents from API", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => ({
        torrents: [
          {
            id: 1,
            name: "Ubuntu 24.04 LTS",
            size: 4294967296,
            seeders: 10,
            leechers: 5,
            uploader_id: 1,
            uploader: "admin",
            banned: false,
            created_at: "2024-05-01T00:00:00Z",
          },
        ],
        total: 1,
      }),
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Ubuntu 24.04 LTS")).toBeInTheDocument();
    });
    expect(screen.getByText("admin")).toBeInTheDocument();
    expect(screen.getByText("Active")).toBeInTheDocument();
  });

  test("displays banned badge for banned torrents", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => ({
        torrents: [
          {
            id: 2,
            name: "Banned Torrent",
            size: 1024,
            seeders: 0,
            leechers: 0,
            uploader_id: 1,
            uploader: "baduser",
            banned: true,
            created_at: "2024-05-01T00:00:00Z",
          },
        ],
        total: 1,
      }),
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Banned")).toBeInTheDocument();
    });
  });

  test("renders search input", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => ({ torrents: [], total: 0 }),
    });

    renderPage();

    expect(
      screen.getByPlaceholderText("Torrent name or uploader..."),
    ).toBeInTheDocument();
  });

  test("renders loading state initially", () => {
    mockFetch.mockReturnValue(new Promise(() => {}));

    renderPage();

    expect(screen.getByText("Loading...")).toBeInTheDocument();
  });

  test("renders delete button for each torrent", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => ({
        torrents: [
          {
            id: 1,
            name: "Test Torrent",
            size: 1024,
            seeders: 1,
            leechers: 0,
            uploader_id: 1,
            uploader: "uploader",
            banned: false,
            created_at: "2024-05-01T00:00:00Z",
          },
        ],
        total: 1,
      }),
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Delete")).toBeInTheDocument();
    });
  });
});
