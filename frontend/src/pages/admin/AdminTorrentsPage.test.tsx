import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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

const HASH_A = "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0";

/** One row of the admin listing, matching what the backend now sends. */
function torrentRow(overrides: Record<string, unknown> = {}) {
  return {
    id: 1,
    name: "Ubuntu 24.04 LTS",
    info_hash: HASH_A,
    size: 4294967296,
    seeders: 10,
    leechers: 5,
    uploader_id: 1,
    uploader: "admin",
    banned: false,
    free: false,
    silver: false,
    hnr_exempt: false,
    visible: true,
    created_at: "2024-05-01T00:00:00Z",
    ...overrides,
  };
}

/**
 * Routes the mocked fetch by URL and method so the listing reload after a mutation
 * returns whatever the test wants next. `bulk` is the response to the bulk POST.
 */
function mockApi(options: {
  torrents?: Record<string, unknown>[];
  total?: number;
  bulk?: unknown;
  deleteOk?: boolean;
}) {
  mockFetch.mockImplementation((url: string, init?: { method?: string }) => {
    const method = init?.method ?? "GET";

    if (method === "POST" && url.includes("/bulk")) {
      return Promise.resolve(
        options.bulk ?? {
          ok: true,
          json: async () => ({
            action: "ban",
            results: [],
            succeeded: 0,
            failed: 0,
          }),
        },
      );
    }
    if (method === "DELETE") {
      return Promise.resolve({
        ok: options.deleteOk ?? true,
        json: async () => ({}),
      });
    }
    return Promise.resolve({
      ok: true,
      json: async () => ({
        torrents: options.torrents ?? [],
        total: options.total ?? options.torrents?.length ?? 0,
      }),
    });
  });
}

function renderPage() {
  return render(
    <ToastProvider>
      <MemoryRouter initialEntries={["/admin/torrents"]}>
        <AdminTorrentsPage />
      </MemoryRouter>
    </ToastProvider>,
  );
}

/** The query string of the most recent listing request. */
function lastListingURL(): string {
  const calls = mockFetch.mock.calls.filter(
    (c) => (c[1]?.method ?? "GET") === "GET",
  );
  return String(calls[calls.length - 1]?.[0] ?? "");
}

describe("AdminTorrentsPage", () => {
  test("renders page title", async () => {
    mockApi({});
    renderPage();
    expect(screen.getByText("Torrents")).toBeInTheDocument();
  });

  test("shows empty state when no torrents exist", async () => {
    mockApi({});
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("No torrents found.")).toBeInTheDocument();
    });
  });

  test("displays torrents from API", async () => {
    mockApi({ torrents: [torrentRow()] });
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Ubuntu 24.04 LTS")).toBeInTheDocument();
    });
    expect(screen.getByText("admin")).toBeInTheDocument();
    expect(screen.getByText("Active")).toBeInTheDocument();
  });

  test("displays banned badge for banned torrents", async () => {
    mockApi({ torrents: [torrentRow({ id: 2, banned: true })] });
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Banned")).toBeInTheDocument();
    });
  });

  // The whole hash has to be in the DOM, not just a tooltip: copying it back out
  // into a report or a reply to a takedown is why the column exists, and tooltip
  // text cannot be selected. CSS does the truncating.
  test("renders the full info hash so it can be copied", async () => {
    mockApi({ torrents: [torrentRow()] });
    renderPage();

    await waitFor(() => {
      expect(screen.getByText(HASH_A)).toBeInTheDocument();
    });
    expect(screen.getByText(HASH_A)).toHaveAttribute("title", HASH_A);
  });

  test("shows the freeleech and half-credit flags", async () => {
    mockApi({ torrents: [torrentRow({ free: true, silver: true })] });
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Free")).toBeInTheDocument();
    });
    expect(screen.getByText("Half")).toBeInTheDocument();
  });

  test("shows the hit-and-run exemption flag", async () => {
    mockApi({ torrents: [torrentRow({ hnr_exempt: true })] });
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("No H&R")).toBeInTheDocument();
    });
  });

  test("does not show the hit-and-run exemption flag for a non-exempt torrent", async () => {
    mockApi({ torrents: [torrentRow({ hnr_exempt: false })] });
    renderPage();

    await waitFor(() => {
      expect(screen.getByText(HASH_A)).toBeInTheDocument();
    });
    expect(screen.queryByText("No H&R")).not.toBeInTheDocument();
  });

  test("renders search input", async () => {
    mockApi({});
    renderPage();

    expect(
      screen.getByPlaceholderText("Torrent name or info hash..."),
    ).toBeInTheDocument();
  });

  // The hint is the whole affordance: without it, pasting a hash into a box
  // labelled "search" looks like it is about to do a name search and find nothing.
  test("says when a search term is being treated as an info hash", async () => {
    mockApi({});
    renderPage();

    const box = screen.getByPlaceholderText("Torrent name or info hash...");
    await userEvent.type(box, HASH_A);

    await waitFor(() => {
      expect(screen.getByText("Looking up by info hash.")).toBeInTheDocument();
    });

    await userEvent.clear(box);
    await userEvent.type(box, "not a hash");
    await waitFor(() => {
      expect(
        screen.queryByText("Looking up by info hash."),
      ).not.toBeInTheDocument();
    });
  });

  test("renders loading state initially", () => {
    mockFetch.mockReturnValue(new Promise(() => {}));
    renderPage();
    expect(screen.getByText("Loading...")).toBeInTheDocument();
  });

  test("renders delete button for each torrent", async () => {
    mockApi({ torrents: [torrentRow({ name: "Test Torrent" })] });
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Delete")).toBeInTheDocument();
    });
  });

  test("filters by banned status", async () => {
    mockApi({ torrents: [torrentRow()] });
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Ubuntu 24.04 LTS")).toBeInTheDocument();
    });

    await userEvent.selectOptions(
      screen.getByLabelText("Status"),
      screen.getByRole("option", { name: "Banned" }),
    );

    await waitFor(() => {
      expect(lastListingURL()).toContain("banned=true");
    });

    await userEvent.selectOptions(
      screen.getByLabelText("Status"),
      screen.getByRole("option", { name: "Not banned" }),
    );
    await waitFor(() => {
      expect(lastListingURL()).toContain("banned=false");
    });

    // "All" must drop the filter rather than send an empty value the API would
    // reject.
    await userEvent.selectOptions(
      screen.getByLabelText("Status"),
      screen.getByRole("option", { name: "All" }),
    );
    await waitFor(() => {
      expect(lastListingURL()).not.toContain("banned=");
    });
  });

  // --- bulk actions ---

  test("the bulk bar appears only once rows are selected", async () => {
    mockApi({
      torrents: [torrentRow(), torrentRow({ id: 2, name: "Second" })],
    });
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Ubuntu 24.04 LTS")).toBeInTheDocument();
    });
    expect(screen.queryByText(/selected/)).not.toBeInTheDocument();

    await userEvent.click(screen.getByLabelText("Select Ubuntu 24.04 LTS"));

    expect(screen.getByText("1 selected")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Ban selected" }),
    ).toBeInTheDocument();
  });

  test("select-all ticks every row on the page", async () => {
    mockApi({
      torrents: [torrentRow(), torrentRow({ id: 2, name: "Second" })],
    });
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Second")).toBeInTheDocument();
    });

    await userEvent.click(screen.getByLabelText("Select all on this page"));
    expect(screen.getByText("2 selected")).toBeInTheDocument();

    await userEvent.click(screen.getByLabelText("Select all on this page"));
    expect(screen.queryByText("2 selected")).not.toBeInTheDocument();
  });

  test("banning selected torrents posts the ids and reports the count", async () => {
    mockApi({
      torrents: [torrentRow(), torrentRow({ id: 2, name: "Second" })],
      bulk: {
        ok: true,
        json: async () => ({
          action: "ban",
          results: [
            { id: 1, status: "ok" },
            { id: 2, status: "ok" },
          ],
          succeeded: 2,
          failed: 0,
        }),
      },
    });
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Second")).toBeInTheDocument();
    });

    await userEvent.click(screen.getByLabelText("Select all on this page"));
    await userEvent.click(screen.getByRole("button", { name: "Ban selected" }));
    await userEvent.click(screen.getByRole("button", { name: "Ban" }));

    await waitFor(() => {
      expect(screen.getByText("2 torrents banned")).toBeInTheDocument();
    });

    const post = mockFetch.mock.calls.find(
      (c) => (c[1]?.method ?? "GET") === "POST",
    );
    expect(String(post?.[0])).toContain("/api/v1/admin/torrents/bulk");
    expect(JSON.parse(String(post?.[1]?.body))).toEqual({
      action: "ban",
      ids: [1, 2],
    });
  });

  // A 200 with failures is normal. Reporting only the successes would tell an
  // operator their ban worked when part of it did not.
  test("reports why some torrents in a bulk action failed", async () => {
    mockApi({
      torrents: [torrentRow(), torrentRow({ id: 2, name: "Second" })],
      bulk: {
        ok: true,
        json: async () => ({
          action: "delete",
          results: [
            { id: 1, status: "ok" },
            { id: 2, status: "not_found" },
          ],
          succeeded: 1,
          failed: 1,
        }),
      },
    });
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Second")).toBeInTheDocument();
    });

    await userEvent.click(screen.getByLabelText("Select all on this page"));
    await userEvent.click(
      screen.getByRole("button", { name: "Delete selected" }),
    );
    // The row buttons and the modal share the "Delete" label; the modal's is last.
    const confirms = screen.getAllByRole("button", { name: "Delete" });
    await userEvent.click(confirms[confirms.length - 1]);

    await waitFor(() => {
      expect(
        screen.getByText("1 deleted, 1 could not be: 1 not found"),
      ).toBeInTheDocument();
    });
  });

  test("a rejected bulk request surfaces the server's reason", async () => {
    mockApi({
      torrents: [torrentRow()],
      bulk: {
        ok: false,
        json: async () => ({
          error: {
            code: "bad_request",
            message: "at most 100 torrents per request",
          },
        }),
      },
    });
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Ubuntu 24.04 LTS")).toBeInTheDocument();
    });

    await userEvent.click(screen.getByLabelText("Select all on this page"));
    await userEvent.click(screen.getByRole("button", { name: "Ban selected" }));
    await userEvent.click(screen.getByRole("button", { name: "Ban" }));

    await waitFor(() => {
      expect(
        screen.getByText("at most 100 torrents per request"),
      ).toBeInTheDocument();
    });
  });

  // A tick left over after the list moved on would act on a torrent the operator
  // can no longer see.
  test("clears the selection when the filter changes", async () => {
    mockApi({ torrents: [torrentRow()] });
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Ubuntu 24.04 LTS")).toBeInTheDocument();
    });

    await userEvent.click(screen.getByLabelText("Select all on this page"));
    expect(screen.getByText("1 selected")).toBeInTheDocument();

    await userEvent.selectOptions(
      screen.getByLabelText("Status"),
      screen.getByRole("option", { name: "Banned" }),
    );

    await waitFor(() => {
      expect(screen.queryByText("1 selected")).not.toBeInTheDocument();
    });
  });

  test("the unban confirmation is not styled as destructive", async () => {
    mockApi({ torrents: [torrentRow({ banned: true })] });
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Banned")).toBeInTheDocument();
    });

    await userEvent.click(screen.getByLabelText("Select all on this page"));
    await userEvent.click(
      screen.getByRole("button", { name: "Unban selected" }),
    );

    expect(screen.getByText("Unban 1 Torrent")).toBeInTheDocument();
    expect(
      screen.getByText(/They become downloadable again/),
    ).toBeInTheDocument();
    // The styling, not just the copy: unban is the one action here that undoes
    // something, and a red button says the opposite.
    const confirm = screen.getByRole("button", { name: "Unban" });
    expect(confirm).toHaveClass("modal-btn--primary");
    expect(confirm).not.toHaveClass("modal-btn--danger");
  });

  // A failed request must not answer the operator's question with "no torrents".
  // With the banned filter applied that is a believable wrong answer they will act
  // on — "there are no banned torrents" — rather than an obvious breakage.
  test("a failed listing is distinguished from an empty one", async () => {
    mockFetch.mockResolvedValue({ ok: false, json: async () => ({}) });
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Couldn't load torrents.")).toBeInTheDocument();
    });
    expect(screen.queryByText("No torrents found.")).not.toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Try again" }),
    ).toBeInTheDocument();
  });

  test("a network failure is reported too", async () => {
    mockFetch.mockRejectedValue(new Error("offline"));
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Couldn't load torrents.")).toBeInTheDocument();
    });
  });

  // A hash identifies exactly one torrent, so ANDing a status predicate with it can
  // only hide the answer — a takedown lookup with Status left on "Banned" would
  // report "not on this tracker" for a torrent that is.
  test("a hash lookup ignores the status filter", async () => {
    mockApi({ torrents: [torrentRow()] });
    render(
      <ToastProvider>
        <MemoryRouter
          initialEntries={[`/admin/torrents?q=${HASH_A}&status=banned`]}
        >
          <AdminTorrentsPage />
        </MemoryRouter>
      </ToastProvider>,
    );

    await waitFor(() => {
      expect(lastListingURL()).toContain("search=" + HASH_A);
    });
    expect(lastListingURL()).not.toContain("banned=");
  });

  // An unrecognised status in the URL used to send `banned=false` while the Select
  // displayed "All": the UI said one thing and the query did another.
  test("an unrecognised status in the URL is ignored, not guessed at", async () => {
    mockApi({ torrents: [torrentRow()] });
    render(
      <ToastProvider>
        <MemoryRouter initialEntries={["/admin/torrents?status=Banned"]}>
          <AdminTorrentsPage />
        </MemoryRouter>
      </ToastProvider>,
    );

    await waitFor(() => {
      expect(screen.getByText("Ubuntu 24.04 LTS")).toBeInTheDocument();
    });
    expect(lastListingURL()).not.toContain("banned=");
    expect(screen.getByLabelText("Status")).toHaveValue("");
  });

  // The request already dropped the status predicate during a hash lookup, but the
  // Select went on rendering "Banned" as selected. An operator would then read an
  // Active row as proof the torrent is banned — the same
  // UI-says-one-thing-query-does-another defect this page fixed for ?status=, one
  // line away. The control has to show that it is not being applied.
  test("the status control shows it is not applied during a hash lookup", async () => {
    mockApi({ torrents: [torrentRow()] });
    render(
      <ToastProvider>
        <MemoryRouter
          initialEntries={[`/admin/torrents?q=${HASH_A}&status=banned`]}
        >
          <AdminTorrentsPage />
        </MemoryRouter>
      </ToastProvider>,
    );

    await waitFor(() => {
      expect(lastListingURL()).toContain("search=" + HASH_A);
    });
    const select = screen.getByLabelText("Status");
    expect(select).toBeDisabled();
    expect(select).toHaveValue("");
    expect(screen.getByText(/ignoring the status filter/i)).toBeInTheDocument();
  });

  // The backend's `name=` escape hatch had no route to it from the only client:
  // the page sent `search=` and nothing else, so a release genuinely titled like a
  // SHA1 was permanently unfindable by name here — the opposite of what the escape
  // hatch is for.
  test("a hash-shaped term can be searched by name instead", async () => {
    mockApi({ torrents: [torrentRow()] });
    render(
      <ToastProvider>
        <MemoryRouter initialEntries={[`/admin/torrents?q=${HASH_A}`]}>
          <AdminTorrentsPage />
        </MemoryRouter>
      </ToastProvider>,
    );

    await waitFor(() => {
      expect(lastListingURL()).toContain("search=" + HASH_A);
    });

    await userEvent.click(
      screen.getByRole("button", { name: /search by name instead/i }),
    );

    await waitFor(() => {
      expect(lastListingURL()).toContain("name=" + HASH_A);
    });
    // `search=` would let the backend route it straight back to the hash.
    expect(lastListingURL()).not.toContain("search=");
    // And the status filter is available again, because a name search can return many rows.
    expect(screen.getByLabelText("Status")).not.toBeDisabled();
  });

  test("a forced name search can be switched back to the hash lookup", async () => {
    mockApi({ torrents: [torrentRow()] });
    render(
      <ToastProvider>
        <MemoryRouter initialEntries={[`/admin/torrents?q=${HASH_A}&by=name`]}>
          <AdminTorrentsPage />
        </MemoryRouter>
      </ToastProvider>,
    );

    await waitFor(() => {
      expect(lastListingURL()).toContain("name=" + HASH_A);
    });

    await userEvent.click(
      screen.getByRole("button", { name: /look up by info hash instead/i }),
    );

    await waitFor(() => {
      expect(lastListingURL()).toContain("search=" + HASH_A);
    });
    expect(lastListingURL()).not.toContain("name=");
  });

  test("deleting a single torrent still works", async () => {
    mockApi({ torrents: [torrentRow()] });
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Ubuntu 24.04 LTS")).toBeInTheDocument();
    });

    await userEvent.click(screen.getByRole("button", { name: "Delete" }));
    // The row button and the modal button share a label; the modal's is last.
    const confirms = screen.getAllByRole("button", { name: "Delete" });
    await userEvent.click(confirms[confirms.length - 1]);

    await waitFor(() => {
      expect(screen.getByText("Torrent deleted")).toBeInTheDocument();
    });
    const del = mockFetch.mock.calls.find(
      (c) => (c[1]?.method ?? "GET") === "DELETE",
    );
    expect(String(del?.[0])).toContain("/api/v1/admin/torrents/1");
  });
});
