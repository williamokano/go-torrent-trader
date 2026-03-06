import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, test, expect, vi } from "vitest";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { TorrentDetailPage } from "@/pages/TorrentDetailPage";

const mockGET = vi.fn();

vi.mock("@/api", () => ({
  api: {
    GET: (...args: unknown[]) => mockGET(...args),
  },
}));

vi.mock("@/features/auth/token", () => ({
  getAccessToken: () => "fake-token",
}));

vi.mock("@/config", () => ({
  getConfig: () => ({ API_URL: "http://localhost:8080", SITE_NAME: "Test" }),
}));

const FAKE_TORRENT = {
  id: 1,
  name: "Ubuntu 24.04 LTS Desktop",
  info_hash: "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0",
  size: 4_800_000_000,
  description: "The latest Ubuntu release.",
  category_id: 1,
  uploader_id: 1,
  anonymous: false,
  seeders: 42,
  leechers: 5,
  times_completed: 318,
  comments_count: 12,
  file_count: 3,
  created_at: "2026-03-05T14:30:00Z",
  updated_at: "2026-03-05T14:30:00Z",
};

afterEach(cleanup);

beforeEach(() => {
  vi.clearAllMocks();
  mockGET.mockResolvedValue({
    data: { torrent: FAKE_TORRENT },
    error: undefined,
  });
});

function renderDetailPage(id = "1") {
  return render(
    <MemoryRouter initialEntries={[`/torrent/${id}`]}>
      <Routes>
        <Route path="/torrent/:id" element={<TorrentDetailPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("TorrentDetailPage", () => {
  test("shows loading state initially", () => {
    mockGET.mockReturnValue(new Promise(() => {}));
    renderDetailPage();
    expect(screen.getByText("Loading torrent...")).toBeInTheDocument();
  });

  test("renders torrent name after loading", async () => {
    renderDetailPage();
    await waitFor(() => {
      expect(screen.getByText("Ubuntu 24.04 LTS Desktop")).toBeInTheDocument();
    });
  });

  test("renders info hash", async () => {
    renderDetailPage();
    await waitFor(() => {
      expect(
        screen.getByText("a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0"),
      ).toBeInTheDocument();
    });
  });

  test("renders category label", async () => {
    renderDetailPage();
    await waitFor(() => {
      expect(screen.getByText("Linux ISOs")).toBeInTheDocument();
    });
  });

  test("renders seeders and leechers stats", async () => {
    renderDetailPage();
    await waitFor(() => {
      expect(screen.getByText("Seeders")).toBeInTheDocument();
    });
    expect(screen.getByText("Leechers")).toBeInTheDocument();
    expect(screen.getByText("42")).toBeInTheDocument();
    expect(screen.getByText("5")).toBeInTheDocument();
  });

  test("renders times completed stat", async () => {
    renderDetailPage();
    await waitFor(() => {
      expect(screen.getByText("Completed")).toBeInTheDocument();
    });
    expect(screen.getByText("318")).toBeInTheDocument();
  });

  test("renders file count", async () => {
    renderDetailPage();
    await waitFor(() => {
      expect(screen.getByText("3")).toBeInTheDocument();
    });
  });

  test("renders description when present", async () => {
    renderDetailPage();
    await waitFor(() => {
      expect(
        screen.getByText("The latest Ubuntu release."),
      ).toBeInTheDocument();
    });
    expect(screen.getByText("Description")).toBeInTheDocument();
  });

  test("does not render description section when absent", async () => {
    mockGET.mockResolvedValue({
      data: { torrent: { ...FAKE_TORRENT, description: undefined } },
      error: undefined,
    });
    renderDetailPage();
    await waitFor(() => {
      expect(screen.getByText("Ubuntu 24.04 LTS Desktop")).toBeInTheDocument();
    });
    expect(screen.queryByText("Description")).not.toBeInTheDocument();
  });

  test("renders download button", async () => {
    renderDetailPage();
    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "Download .torrent" }),
      ).toBeInTheDocument();
    });
  });

  test("shows error on API failure", async () => {
    mockGET.mockResolvedValue({
      data: undefined,
      error: { error: { message: "Torrent not found" } },
    });
    renderDetailPage();
    await waitFor(() => {
      expect(screen.getByText("Torrent not found")).toBeInTheDocument();
    });
  });

  test("shows error for invalid torrent ID", async () => {
    renderDetailPage("abc");
    await waitFor(() => {
      expect(screen.getByText("Invalid torrent ID")).toBeInTheDocument();
    });
    expect(mockGET).not.toHaveBeenCalled();
  });

  test("renders health indicator", async () => {
    renderDetailPage();
    await waitFor(() => {
      expect(screen.getByText("Ubuntu 24.04 LTS Desktop")).toBeInTheDocument();
    });
    const healthDot = document.querySelector(".torrent-detail__health");
    expect(healthDot).not.toBeNull();
    expect(healthDot?.classList.contains("torrent-detail__health--good")).toBe(
      true,
    );
  });

  test("passes authorization header to API", async () => {
    renderDetailPage();
    await waitFor(() => {
      expect(mockGET).toHaveBeenCalledWith(
        "/api/v1/torrents/{id}",
        expect.objectContaining({
          headers: { Authorization: "Bearer fake-token" },
        }),
      );
    });
  });
});
