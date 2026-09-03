import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, test, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { HitAndRunPage } from "@/pages/HitAndRunPage";
import { ToastProvider } from "@/components/toast";

const mockFetch = vi.fn();
vi.stubGlobal("fetch", mockFetch);

const mockRefreshUser = vi.fn();
let mockBalance = 1000;

vi.mock("@/features/auth", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/features/auth")>();
  return {
    ...actual,
    useAuth: () => ({
      user: { id: 1, username: "member", bonus_points: mockBalance },
      isAuthenticated: true,
      isLoading: false,
      refreshUser: mockRefreshUser,
    }),
  };
});

function renderPage() {
  return render(
    <MemoryRouter>
      <ToastProvider>
        <HitAndRunPage />
      </ToastProvider>
    </MemoryRouter>,
  );
}

afterEach(() => {
  cleanup();
  mockFetch.mockReset();
  mockRefreshUser.mockReset();
  mockBalance = 1000;
  vi.useRealTimers();
});

function mockRecords(records: unknown[]) {
  mockFetch.mockResolvedValue({
    ok: true,
    json: async () => ({ records }),
  });
}

describe("HitAndRunPage", () => {
  test("shows the empty state when there is nothing tracked", async () => {
    mockRecords([]);
    renderPage();
    expect(await screen.findByText(/nothing tracked yet/i)).toBeInTheDocument();
  });

  test("shows an error when the request fails", async () => {
    mockFetch.mockResolvedValue({ ok: false, json: async () => ({}) });
    renderPage();
    expect(
      await screen.findByText(/failed to load hit-and-run records/i),
    ).toBeInTheDocument();
  });

  test("puts a breach record under In breach with its badge", async () => {
    mockRecords([
      {
        id: 1,
        torrent_id: 100,
        torrent_name: "Breached Release",
        torrent_size: 1_000_000_000,
        state: "hnr",
        display_status: "breach",
        completed_at: new Date().toISOString(),
        last_seen_at: new Date(Date.now() - 3 * 3600_000).toISOString(),
        breached_at: new Date(Date.now() - 3600_000).toISOString(),
        seeded_seconds: 100,
        uploaded: 500,
        currently_seeding: false,
        required_seed_hours: 240,
        required_ratio: 1.0,
      },
    ]);
    renderPage();

    const heading = await screen.findByText(/in breach \(1\)/i);
    const section = heading.closest("section")!;
    expect(within(section).getByText("Breached Release")).toBeInTheDocument();
    expect(within(section).getByText("In breach")).toBeInTheDocument();
  });

  test("shows seed-time and upload progress for a monitored record", async () => {
    mockRecords([
      {
        id: 2,
        torrent_id: 200,
        torrent_name: "Still Seeding",
        torrent_size: 1_073_741_824, // 1 GiB
        state: "active",
        display_status: "monitoring",
        completed_at: new Date().toISOString(),
        last_seen_at: new Date().toISOString(),
        seeded_seconds: 3600 * 12, // 12h of 24h required
        uploaded: 536_870_912, // 0.5 GiB of 1 GiB required (ratio 1.0)
        currently_seeding: true,
        required_seed_hours: 24,
        required_ratio: 1.0,
      },
    ]);
    renderPage();

    const heading = await screen.findByText(/seeding in progress \(1\)/i);
    const section = heading.closest("section")!;
    expect(within(section).getByText("Still Seeding")).toBeInTheDocument();
    expect(within(section).getByText("Seeding now")).toBeInTheDocument();
    expect(
      within(section).getByText(/12\.0h.*24\.0h.*50%/),
    ).toBeInTheDocument();
    expect(
      within(section).getByText(/512\.00 MB.*1\.00 GB.*50%/),
    ).toBeInTheDocument();
    expect(within(section).getByText(/projected clear/i)).toBeInTheDocument();
  });

  test("shows 'clock stopped' when a monitored record is not currently seeding", async () => {
    mockRecords([
      {
        id: 3,
        torrent_id: 300,
        torrent_name: "Paused",
        torrent_size: 1_000_000_000,
        state: "active",
        display_status: "monitoring",
        completed_at: new Date().toISOString(),
        last_seen_at: new Date().toISOString(),
        seeded_seconds: 100,
        uploaded: 0,
        currently_seeding: false,
        required_seed_hours: 240,
        required_ratio: 1.0,
      },
    ]);
    renderPage();

    expect(await screen.findByText("Not seeding")).toBeInTheDocument();
    expect(
      screen.getByText(/clock stopped — not currently seeding/i),
    ).toBeInTheDocument();
  });

  test("lists resolved records with their outcome label", async () => {
    mockRecords([
      {
        id: 4,
        torrent_id: 400,
        torrent_name: "Done Deal",
        torrent_size: 1000,
        state: "satisfied",
        display_status: "satisfied",
        completed_at: new Date().toISOString(),
        last_seen_at: new Date().toISOString(),
        resolved_at: new Date().toISOString(),
        seeded_seconds: 100000,
        uploaded: 1000,
        currently_seeding: false,
      },
      {
        id: 5,
        torrent_id: 401,
        torrent_name: "Paid Off",
        torrent_size: 1000,
        state: "cleared",
        display_status: "cleared",
        completed_at: new Date().toISOString(),
        last_seen_at: new Date().toISOString(),
        resolved_at: new Date().toISOString(),
        seeded_seconds: 10,
        uploaded: 0,
        currently_seeding: false,
      },
    ]);
    renderPage();

    expect(await screen.findByText("Done Deal")).toBeInTheDocument();
    expect(screen.getByText("Satisfied")).toBeInTheDocument();
    expect(screen.getByText("Paid Off")).toBeInTheDocument();
    expect(screen.getByText("Cleared with points")).toBeInTheDocument();
  });

  test("sends the bearer token when present", async () => {
    localStorage.setItem("torrenttrader-access-token", "test-token-123");
    mockRecords([]);
    renderPage();
    await screen.findByText(/nothing tracked yet/i);

    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining("/api/v1/hnr"),
      expect.objectContaining({
        headers: expect.objectContaining({
          Authorization: "Bearer test-token-123",
        }),
      }),
    );
    localStorage.removeItem("torrenttrader-access-token");
  });
});

describe("HitAndRunPage clearing with points", () => {
  interface FetchInit {
    method?: string;
  }

  function mockClearApi(options?: {
    breachRecord?: Record<string, unknown>;
    price?: number;
    clearOk?: boolean;
    clearBody?: Record<string, unknown>;
  }) {
    const price = options?.price ?? 60;
    const calls: { method: string; url: string }[] = [];
    const record = options?.breachRecord ?? {
      id: 1,
      torrent_id: 100,
      torrent_name: "Breached Release",
      torrent_size: 1_000_000_000,
      state: "hnr",
      display_status: "breach",
      completed_at: new Date().toISOString(),
      last_seen_at: new Date().toISOString(),
      breached_at: new Date().toISOString(),
      seeded_seconds: 100,
      uploaded: 500,
      currently_seeding: false,
    };
    mockFetch.mockImplementation((url: string, init?: FetchInit) => {
      const method = init?.method ?? "GET";
      calls.push({ method, url });
      if (method === "GET" && url.endsWith("/api/v1/hnr")) {
        return Promise.resolve({
          ok: true,
          json: async () => ({ records: [record] }),
        });
      }
      if (method === "GET" && url.includes("/clear-price")) {
        return Promise.resolve({ ok: true, json: async () => ({ price }) });
      }
      if (method === "POST" && url.includes("/clear-all")) {
        return Promise.resolve({
          ok: true,
          json: async () => ({
            cleared: 1,
            total_spent: price,
            new_balance: 1000 - price,
            stopped_insufficient_points: false,
            ...options?.clearBody,
          }),
        });
      }
      if (method === "POST" && url.includes("/clear")) {
        if (options?.clearOk === false) {
          return Promise.resolve({
            ok: false,
            json: async () => ({ error: { message: "insufficient points" } }),
          });
        }
        return Promise.resolve({
          ok: true,
          json: async () => ({
            price,
            new_balance: 1000 - price,
            ...options?.clearBody,
          }),
        });
      }
      return Promise.resolve({ ok: true, json: async () => ({}) });
    });
    return calls;
  }

  test("shows the quoted price on the clear button for an open record", async () => {
    mockClearApi({ price: 60 });
    renderPage();

    expect(
      await screen.findByRole("button", { name: "Clear for 60 pts" }),
    ).toBeInTheDocument();
  });

  test("clearing a record posts to its clear endpoint and refreshes", async () => {
    const calls = mockClearApi({ price: 60 });
    const user = userEvent.setup();
    renderPage();

    const button = await screen.findByRole("button", {
      name: "Clear for 60 pts",
    });
    await user.click(button);

    expect(await screen.findByText("Cleared for 60 pts")).toBeInTheDocument();
    expect(
      calls.some((c) => c.method === "POST" && c.url.endsWith("/1/clear")),
    ).toBe(true);
    expect(mockRefreshUser).toHaveBeenCalled();
  });

  test("the clear button is disabled when the balance can't cover the price", async () => {
    mockBalance = 10;
    mockClearApi({ price: 60 });
    renderPage();

    expect(
      await screen.findByRole("button", { name: "Clear for 60 pts" }),
    ).toBeDisabled();
  });

  test("clear all affordable posts to the clear-all endpoint", async () => {
    const calls = mockClearApi({ price: 60 });
    const user = userEvent.setup();
    renderPage();

    const button = await screen.findByRole("button", {
      name: "Clear all affordable",
    });
    await user.click(button);

    expect(await screen.findByText("Cleared 1 for 60 pts")).toBeInTheDocument();
    expect(
      calls.some((c) => c.method === "POST" && c.url.endsWith("/clear-all")),
    ).toBe(true);
  });

  test("a failed clear shows an error and does not refresh the balance", async () => {
    mockClearApi({ price: 60, clearOk: false });
    const user = userEvent.setup();
    renderPage();

    const button = await screen.findByRole("button", {
      name: "Clear for 60 pts",
    });
    await user.click(button);

    expect(await screen.findByText("insufficient points")).toBeInTheDocument();
    expect(mockRefreshUser).not.toHaveBeenCalled();
  });
});
