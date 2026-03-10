import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { AdminDashboardPage } from "@/pages/admin/AdminDashboardPage";

const mockFetch = vi.fn();

vi.stubGlobal("fetch", mockFetch);

afterEach(cleanup);

beforeEach(() => {
  vi.clearAllMocks();
});

function renderPage() {
  return render(
    <MemoryRouter initialEntries={["/admin"]}>
      <AdminDashboardPage />
    </MemoryRouter>,
  );
}

const dashboardResponse = {
  users: { total: 150, today: 3, week: 12 },
  torrents: { total: 500, today: 7 },
  peers: { total: 80, seeders: 60, leechers: 20 },
  pending_reports: 2,
  active_warnings: 5,
  active_mutes: 1,
  recent_activity: [
    {
      id: 1,
      event_type: "user.registered",
      actor_id: 42,
      message: "User testuser registered",
      created_at: new Date().toISOString(),
    },
    {
      id: 2,
      event_type: "torrent.uploaded",
      actor_id: null,
      message: "Torrent uploaded",
      created_at: new Date().toISOString(),
    },
  ],
};

describe("AdminDashboardPage", () => {
  test("renders loading state initially", () => {
    mockFetch.mockReturnValue(new Promise(() => {}));
    renderPage();
    expect(screen.getByText("Loading dashboard...")).toBeInTheDocument();
  });

  test("renders dashboard stats after loading", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => dashboardResponse,
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Dashboard")).toBeInTheDocument();
    });

    expect(screen.getByText("150")).toBeInTheDocument();
    expect(screen.getByText("500")).toBeInTheDocument();
    expect(screen.getByText("80")).toBeInTheDocument();
    expect(screen.getByText("2")).toBeInTheDocument();
    expect(screen.getByText("5")).toBeInTheDocument();
    expect(screen.getByText("1")).toBeInTheDocument();
  });

  test("renders stat card labels", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => dashboardResponse,
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Users")).toBeInTheDocument();
    });

    expect(screen.getByText("Torrents")).toBeInTheDocument();
    expect(screen.getByText("Peers")).toBeInTheDocument();
    expect(screen.getByText("Pending Reports")).toBeInTheDocument();
    expect(screen.getByText("Active Warnings")).toBeInTheDocument();
    expect(screen.getByText("Active Mutes")).toBeInTheDocument();
  });

  test("renders sub-stats for users and torrents", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => dashboardResponse,
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("3")).toBeInTheDocument();
    });

    // user week count
    expect(screen.getByText("12")).toBeInTheDocument();
    // torrent today count
    expect(screen.getByText("7")).toBeInTheDocument();
    // peer sub-stats
    expect(screen.getByText("60")).toBeInTheDocument();
    expect(screen.getByText("20")).toBeInTheDocument();
  });

  test("renders recent activity table", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => dashboardResponse,
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Recent Activity")).toBeInTheDocument();
    });

    expect(screen.getByText("user.registered")).toBeInTheDocument();
    expect(screen.getByText("User testuser registered")).toBeInTheDocument();
    expect(screen.getByText("#42")).toBeInTheDocument();
    expect(screen.getByText("System")).toBeInTheDocument();
  });

  test("renders error state on fetch failure", async () => {
    mockFetch.mockResolvedValue({
      ok: false,
      status: 500,
    });

    renderPage();

    await waitFor(() => {
      expect(
        screen.getByText("Failed to load dashboard data"),
      ).toBeInTheDocument();
    });
  });

  test("renders empty activity state", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => ({
        ...dashboardResponse,
        recent_activity: [],
      }),
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("No recent activity.")).toBeInTheDocument();
    });
  });
});
