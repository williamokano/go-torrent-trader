import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { AnnounceLogPanel } from "@/components/AnnounceLogPanel";
import { ToastProvider } from "@/components/toast";

const mockFetch = vi.fn();
vi.stubGlobal("fetch", mockFetch);

const entry = {
  id: 1,
  torrent_id: 7,
  torrent_name: "Some Release",
  event: "started",
  ip: "203.0.113.9",
  port: 6881,
  peer_id: "2d7142ff",
  uploaded: 100,
  downloaded: 200,
  left_bytes: 300,
  uploaded_delta: 1048576,
  downloaded_delta: 2097152,
  counted_downloaded_delta: 1048576,
  seeder: false,
  announced_at: "2026-07-01T12:00:00Z",
};

const deletedTorrentEntry = {
  ...entry,
  id: 2,
  torrent_id: null,
  torrent_name: "Deleted Torrent",
  seeder: true,
};

const period = {
  year_month: "2026-06",
  uploaded: 3221225472,
  downloaded: 1073741824,
  counted_downloaded: 1073741824,
  announces: 42,
  seed_announces: 40,
  ratio: 3,
};

function mockApi(
  body: Partial<{
    events: unknown[];
    monthly: unknown[];
    total: number;
    retention_days: number;
  }> = {},
) {
  mockFetch.mockImplementation((url: string) => {
    if (url.includes("/export")) {
      return Promise.resolve({
        ok: true,
        blob: async () => new Blob(["announced_at\n"]),
      });
    }
    return Promise.resolve({
      ok: true,
      json: async () => ({
        events: body.events ?? [entry, deletedTorrentEntry],
        monthly: body.monthly ?? [period],
        total: body.total ?? 2,
        page: 1,
        per_page: 25,
        retention_days: body.retention_days ?? 90,
      }),
    });
  });
}

function renderPanel(props?: { userId?: number; isOwnLog?: boolean }) {
  return render(
    <MemoryRouter>
      <ToastProvider>
        <AnnounceLogPanel
          userId={props?.userId ?? 42}
          isOwnLog={props?.isOwnLog ?? true}
        />
      </ToastProvider>
    </MemoryRouter>,
  );
}

describe("AnnounceLogPanel", () => {
  beforeEach(() => {
    mockFetch.mockReset();
  });

  afterEach(cleanup);

  test("shows monthly totals and the raw announces", async () => {
    mockApi();
    renderPanel();

    await waitFor(() => {
      expect(screen.getByText("2026-06")).toBeInTheDocument();
    });

    // Monthly figures come from the permanent aggregate, so they must render even
    // for months whose raw rows have been pruned.
    expect(screen.getByText("3.00 GB")).toBeInTheDocument();
    expect(screen.getByText("42")).toBeInTheDocument();

    expect(screen.getByRole("link", { name: "Some Release" })).toHaveAttribute(
      "href",
      "/torrent/7",
    );
    // A deleted torrent has no id to link to, so the name is plain text.
    expect(
      screen.queryByRole("link", { name: "Deleted Torrent" }),
    ).not.toBeInTheDocument();
    expect(screen.getByText("Deleted Torrent")).toBeInTheDocument();
  });

  // The retention window is the member-facing half of the privacy story: without
  // it, an absent month looks like lost history rather than a deletion policy.
  test("states the retention window", async () => {
    mockApi({ retention_days: 30 });
    renderPanel();

    await waitFor(() => {
      expect(
        screen.getByText(/Individual announces are kept for up to 30 days/),
      ).toBeInTheDocument();
    });
  });

  test("says so when pruning is disabled", async () => {
    mockApi({ retention_days: 0 });
    renderPanel();

    await waitFor(() => {
      expect(
        screen.getByText(/kept indefinitely on this site/),
      ).toBeInTheDocument();
    });
  });

  test("wording changes when staff view someone else's log", async () => {
    mockApi();
    renderPanel({ isOwnLog: false });

    await waitFor(() => {
      expect(
        screen.getByText(/this member's client makes is recorded/),
      ).toBeInTheDocument();
    });
  });

  // The export must go through fetch with the bearer token, not a bare href — a
  // link would arrive unauthenticated and 401.
  test("downloads the CSV export through an authenticated fetch", async () => {
    mockApi();
    const createObjectURL = vi.fn(() => "blob:announce-log");
    const revokeObjectURL = vi.fn();
    vi.stubGlobal("URL", { ...URL, createObjectURL, revokeObjectURL });

    renderPanel({ userId: 7 });
    await waitFor(() => {
      expect(screen.getByText("2026-06")).toBeInTheDocument();
    });

    await userEvent.click(screen.getByRole("button", { name: "Download CSV" }));

    await waitFor(() => {
      expect(createObjectURL).toHaveBeenCalled();
    });
    const exportCall = mockFetch.mock.calls.find((c) =>
      String(c[0]).includes("/export"),
    );
    expect(exportCall?.[0]).toContain("/api/v1/users/7/announce-log/export");
    expect(revokeObjectURL).toHaveBeenCalled();
  });

  test("surfaces a failed load instead of rendering an empty log", async () => {
    mockFetch.mockResolvedValue({ ok: false, json: async () => ({}) });
    renderPanel();

    await waitFor(() => {
      expect(
        screen.getByText("Failed to load the announce log"),
      ).toBeInTheDocument();
    });
    expect(screen.queryByText("Monthly totals")).not.toBeInTheDocument();
  });

  // A member with no totals yet must be told why rather than shown a blank table:
  // the rollup runs nightly, so a new account genuinely has nothing.
  test("explains an empty monthly summary", async () => {
    mockApi({ monthly: [], events: [], total: 0 });
    renderPanel();

    await waitFor(() => {
      expect(
        screen.getByText(/No months have been totalled yet/),
      ).toBeInTheDocument();
    });
    expect(screen.getByText("No announces recorded.")).toBeInTheDocument();
  });

  test("pages through the announces", async () => {
    mockApi({ total: 60 });
    renderPanel();

    await waitFor(() => {
      expect(screen.getByText("Page 1 of 3")).toBeInTheDocument();
    });
    expect(screen.getByRole("button", { name: "Previous" })).toBeDisabled();

    await userEvent.click(screen.getByRole("button", { name: "Next" }));

    await waitFor(() => {
      expect(screen.getByText("Page 2 of 3")).toBeInTheDocument();
    });
    const pageTwoCall = mockFetch.mock.calls.find((c) =>
      String(c[0]).includes("page=2"),
    );
    expect(pageTwoCall).toBeDefined();
  });
});
