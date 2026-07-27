import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { userEvent } from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { LiveReleasesPage } from "@/pages/LiveReleasesPage";

let currentToken = "test-token";

vi.mock("@/features/auth/token", () => ({
  getAccessToken: () => currentToken,
}));

vi.mock("@/config", () => ({
  getConfig: () => ({ API_URL: "http://localhost:8080" }),
}));

type Listener = (event: MessageEvent) => void;

/** Stands in for the browser's EventSource, which jsdom does not implement. */
class MockEventSource {
  static instances: MockEventSource[] = [];
  url: string;
  onopen: (() => void) | null = null;
  onerror: (() => void) | null = null;
  closed = false;
  private listeners: Record<string, Listener[]> = {};

  constructor(url: string) {
    this.url = url;
    MockEventSource.instances.push(this);
  }

  addEventListener(type: string, listener: Listener) {
    (this.listeners[type] ??= []).push(listener);
  }

  close() {
    this.closed = true;
  }

  /** Delivers a server frame to the page. */
  emit(type: string, data: unknown) {
    this.emitRaw(type, JSON.stringify(data));
  }

  /** Delivers a frame verbatim, so a test can send something unparseable. */
  emitRaw(type: string, data: string) {
    act(() => {
      for (const listener of this.listeners[type] ?? []) {
        listener({ data } as MessageEvent);
      }
    });
  }

  open() {
    act(() => this.onopen?.());
  }

  fail() {
    act(() => this.onerror?.());
  }
}

function announcement(overrides: Record<string, unknown> = {}) {
  return {
    event: "torrent.published",
    title: "Some.Release-GROUP",
    body: "",
    torrent_id: 42,
    name: "Some.Release-GROUP",
    info_hash: "abc",
    category_id: 3,
    category: "Movies",
    size: 2 * 1024 * 1024 * 1024,
    file_count: 1,
    uploader: "alice",
    anonymous: false,
    freeleech: false,
    silver: false,
    url: "https://tracker.test/torrent/42",
    published_at: new Date().toISOString(),
    ...overrides,
  };
}

/** Answers the feed list; every test gets one so the page never hits the network. */
function mockFeeds(feeds: { slug: string; name: string }[]) {
  vi.stubGlobal(
    "fetch",
    vi.fn(() =>
      Promise.resolve({ ok: true, status: 200, json: async () => ({ feeds }) }),
    ),
  );
}

/** The feed list refusing a member who has no can_feed. */
function mockFeedsForbidden() {
  vi.stubGlobal(
    "fetch",
    vi.fn(() =>
      Promise.resolve({
        ok: false,
        status: 403,
        json: async () => ({ error: { code: "forbidden" } }),
      }),
    ),
  );
}

/** The feed list answering normally first, then refusing — a mid-session revoke. */
function mockFeedsThenForbidden(feeds: { slug: string; name: string }[]) {
  let calls = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn(() => {
      calls += 1;
      if (calls === 1) {
        return Promise.resolve({
          ok: true,
          status: 200,
          json: async () => ({ feeds }),
        });
      }
      return Promise.resolve({
        ok: false,
        status: 403,
        json: async () => ({ error: { code: "forbidden" } }),
      });
    }),
  );
}

beforeEach(() => {
  MockEventSource.instances = [];
  currentToken = "test-token";
  vi.stubGlobal("EventSource", MockEventSource);
  mockFeeds([]);
});

afterEach(cleanup);

function renderPage(initialEntries: string[] = ["/live"]) {
  const result = render(
    <MemoryRouter initialEntries={initialEntries}>
      <LiveReleasesPage />
    </MemoryRouter>,
  );
  return { ...result, source: MockEventSource.instances[0] };
}

describe("LiveReleasesPage", () => {
  test("connects to the stream on the API base URL with the session token", () => {
    renderPage();

    const source = MockEventSource.instances[0];
    expect(source.url).toMatch(/^https?:\/\//);
    expect(source.url).toContain("/api/v1/announce-stream");
    expect(source.url).toContain("token=test-token");
  });

  test("shows an empty state until something arrives", () => {
    renderPage();
    expect(screen.getByText("Waiting for releases…")).toBeInTheDocument();
  });

  test("moves from connecting to live when the stream opens", () => {
    const { source } = renderPage();

    expect(screen.getByRole("status")).toHaveTextContent("connecting");
    source.open();
    expect(screen.getByRole("status")).toHaveTextContent("live");
  });

  // EventSource reconnects on its own, so the page reports the state rather
  // than trying to reconnect itself.
  test("reports reconnecting when the stream drops", () => {
    const { source } = renderPage();

    source.open();
    source.fail();
    expect(screen.getByRole("status")).toHaveTextContent("reconnecting");
  });

  test("renders an announcement with a link, category and size", () => {
    const { source } = renderPage();
    source.open();

    source.emit("announcement", announcement());

    const link = screen.getByRole("link", { name: "Some.Release-GROUP" });
    expect(link).toHaveAttribute("href", "/torrent/42");
    expect(screen.getByText("Movies")).toBeInTheDocument();
    expect(screen.getByText("2.00 GB")).toBeInTheDocument();
    expect(screen.getByText("by alice")).toBeInTheDocument();
  });

  test("marks a freeleech release", () => {
    const { source } = renderPage();
    source.emit("announcement", announcement({ freeleech: true }));

    expect(screen.getByText("Freeleech")).toBeInTheDocument();
  });

  // The anonymity guarantee has to survive all the way to the rendered page.
  test("shows Anonymous rather than a username for an anonymous upload", () => {
    const { source } = renderPage();

    source.emit(
      "announcement",
      announcement({ anonymous: true, uploader: "Anonymous" }),
    );

    expect(screen.getByText("by Anonymous")).toBeInTheDocument();
    expect(screen.queryByText(/alice/)).toBeNull();
  });

  // A reconnect replays whatever the browser missed, which can include an
  // announcement already on screen.
  test("does not render the same torrent twice", () => {
    const { source } = renderPage();

    source.emit("announcement", announcement());
    source.emit("announcement", announcement());

    expect(
      screen.getAllByRole("link", { name: "Some.Release-GROUP" }),
    ).toHaveLength(1);
  });

  test("renders a coalesced batch as a single summary row", () => {
    const { source } = renderPage();

    source.emit("announcement", announcement({ coalesced: 7 }));

    expect(screen.getByText(/7 more releases published/)).toBeInTheDocument();
    // The summary stands for several torrents, so naming one would mislead.
    expect(
      screen.queryByRole("link", { name: "Some.Release-GROUP" }),
    ).toBeNull();
  });

  test("newest first, capped so a page left open does not grow forever", () => {
    const { source } = renderPage();

    for (let i = 1; i <= 120; i++) {
      source.emit(
        "announcement",
        announcement({ torrent_id: i, name: `Release.${i}` }),
      );
    }

    const links = screen.getAllByRole("link");
    expect(links).toHaveLength(100);
    expect(links[0]).toHaveTextContent("Release.120");
    expect(screen.queryByText("Release.20")).toBeNull();
  });

  // A frame the page cannot parse must be skipped, not throw out of the
  // listener and take the feed with it.
  test("a malformed frame does not break the feed", () => {
    const { source } = renderPage();

    source.emitRaw("announcement", "{ this is not json");
    expect(screen.getByText("Waiting for releases…")).toBeInTheDocument();

    source.emit("announcement", announcement());
    expect(
      screen.getByRole("link", { name: "Some.Release-GROUP" }),
    ).toBeInTheDocument();
  });

  test("closes the stream when the page unmounts", () => {
    const { source, unmount } = renderPage();

    unmount();

    expect(source.closed).toBe(true);
  });
  // EventSource's own retry always reuses the URL it was built with — and the
  // token is in that URL. After a refresh the browser would retry a request the
  // server must reject, forever, so the stream is rebuilt with a fresh token.
  test("reconnects with a freshly read token", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      const { source } = renderPage();
      source.open();

      currentToken = "rotated-token";
      source.fail();

      expect(screen.getByRole("status")).toHaveTextContent("reconnecting");
      expect(source.closed).toBe(true);

      await act(async () => {
        await vi.advanceTimersByTimeAsync(6000);
      });

      expect(MockEventSource.instances).toHaveLength(2);
      expect(MockEventSource.instances[1].url).toContain("token=rotated-token");
    } finally {
      vi.useRealTimers();
    }
  });

  test("does not reconnect after unmounting", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      const { source, unmount } = renderPage();
      source.fail();
      unmount();

      await act(async () => {
        await vi.advanceTimersByTimeAsync(30_000);
      });

      expect(MockEventSource.instances).toHaveLength(1);
    } finally {
      vi.useRealTimers();
    }
  });

  // Without pruning, the dedupe set grows for the life of the page even though
  // the visible list is capped.
  test("an announcement evicted by the cap can appear again", () => {
    const { source } = renderPage();

    source.emit("announcement", announcement({ torrent_id: 1, name: "First" }));
    for (let i = 2; i <= 101; i++) {
      source.emit(
        "announcement",
        announcement({ torrent_id: i, name: `Release.${i}` }),
      );
    }
    expect(screen.queryByText("First")).toBeNull();

    source.emit("announcement", announcement({ torrent_id: 1, name: "First" }));
    expect(screen.getByText("First")).toBeInTheDocument();
  });

  // A coalesced summary carries its representative row's torrent id, so keys
  // have to be independent of it or React would see duplicates.
  test("renders several coalesced summaries sharing a torrent id", () => {
    const { source } = renderPage();

    source.emit("announcement", announcement({ coalesced: 3 }));
    source.emit("announcement", announcement({ coalesced: 5 }));

    expect(screen.getByText(/3 more releases published/)).toBeInTheDocument();
    expect(screen.getByText(/5 more releases published/)).toBeInTheDocument();
  });
});

describe("LiveReleasesPage feeds", () => {
  // Note that "default" is first here, and in slug order — so this fixture
  // cannot distinguish "prefers the legacy default" from "takes whatever is
  // first". Tests that turn on that distinction build their own fixture; the
  // rest only need two feeds and use this one.
  const twoFeeds = [
    { slug: "default", name: "Everything" },
    { slug: "no-adult", name: "Safe for work" },
  ];

  test("watches the feed named in the URL", () => {
    renderPage(["/live?feed=no-adult"]);

    expect(MockEventSource.instances[0].url).toBe(
      "http://localhost:8080/api/v1/announce-stream/no-adult?token=test-token",
    );
  });

  test("with no feed list yet it watches the legacy stream", () => {
    // The unslugged route, which is what a link made before feeds existed — or
    // a page open across the deploy — still resolves to. This is also the state
    // the page is in if the feed list cannot be fetched at all.
    renderPage();

    expect(MockEventSource.instances[0].url).toBe(
      "http://localhost:8080/api/v1/announce-stream?token=test-token",
    );
  });

  test("with no feed in the URL it watches the same feed the picker shows", async () => {
    // Regression: the picker used to show the alphabetically-first feed while
    // the stream was bound to the legacy "default" one. On a private tracker
    // that reads "Safe for work" over the unfiltered feed's releases.
    //
    // This needs its own fixture, with "default" NOT first. resolveFeed returns
    // `the legacy default ?? feeds[0]`, so against twoFeeds — where "default" is
    // feeds[0] — both branches give the same answer and the assertion cannot tell
    // a working resolution from one that ignores the legacy default entirely.
    // Mutation-checked: dropping the LEGACY_FEED lookup leaves twoFeeds green.
    mockFeeds([
      { slug: "anime", name: "Anime" },
      { slug: "default", name: "Everything" },
    ]);
    renderPage();

    const picker = (await screen.findByLabelText("Feed")) as HTMLSelectElement;
    // The picker rendering and the stream rebinding onto its feed are separate
    // effects. findByLabelText settles on the first of those, so reading the
    // EventSource here catches the legacy stream whenever the machine is loaded
    // enough to interleave them — which is what running 88 files concurrently
    // does. Settle on the stream, the same way the deleted-feed test below does.
    await waitFor(() => {
      const latest =
        MockEventSource.instances[MockEventSource.instances.length - 1];
      expect(latest.url).toContain(`/announce-stream/${picker.value}?`);
    });
    // "everything" sorts first, but "default" is what the legacy route resolves
    // to, so that is the honest default.
    expect(picker.value).toBe("default");
  });

  test("falls back to the first feed when no feed is named default", async () => {
    mockFeeds([
      { slug: "anime", name: "Anime" },
      { slug: "releases", name: "Releases" },
    ]);
    renderPage();

    const picker = (await screen.findByLabelText("Feed")) as HTMLSelectElement;
    expect(picker.value).toBe("anime");
    // Same two-effect race as above.
    await waitFor(() => {
      const latest =
        MockEventSource.instances[MockEventSource.instances.length - 1];
      expect(latest.url).toContain("/announce-stream/anime?");
    });
  });

  test("says so when the feed in the URL no longer exists", async () => {
    // The admin renamed or deleted it. Silently showing another feed's releases
    // under the requested feed's name is the failure worth avoiding.
    mockFeeds(twoFeeds);
    renderPage(["/live?feed=deleted-feed"]);

    expect(await screen.findByText(/no longer exists/)).toBeInTheDocument();
    // Rendering the message and falling back to the default stream are two
    // separate effects. Reading the latest EventSource the instant the message
    // appears races the reconnect and sometimes catches the previous one, so
    // settle on the stream rather than assuming the message implies it.
    await waitFor(() => {
      const latest =
        MockEventSource.instances[MockEventSource.instances.length - 1];
      expect(latest.url).toContain("/announce-stream/default?");
    });
  });

  test("offers a picker once there is more than one feed", async () => {
    mockFeeds(twoFeeds);
    renderPage();

    const picker = (await screen.findByLabelText("Feed")) as HTMLSelectElement;
    expect(Array.from(picker.options, (option) => option.text)).toEqual([
      "Everything",
      "Safe for work",
    ]);
  });

  test("does not clutter the page with a picker for a single feed", async () => {
    mockFeeds([twoFeeds[0]]);
    renderPage();

    // Settle on the feed fetch itself: "Live Releases" is in the first render,
    // so asserting after it would prove nothing about what the list did.
    await waitFor(() => expect(fetch).toHaveBeenCalled());
    await waitFor(() =>
      expect(MockEventSource.instances.length).toBeGreaterThan(0),
    );
    expect(screen.queryByLabelText("Feed")).not.toBeInTheDocument();
  });

  test("a frame from the feed just left is not shown under the new one", async () => {
    // React runs an effect cleanup after paint, so the old stream is briefly
    // still attached after the switch.
    const user = userEvent.setup();
    mockFeeds(twoFeeds);
    renderPage(["/live?feed=default"]);

    const old = MockEventSource.instances[0];
    const picker = await screen.findByLabelText("Feed");
    await user.selectOptions(picker, "no-adult");

    old.emit("announcement", announcement());

    expect(screen.queryByText("Some.Release-GROUP")).not.toBeInTheDocument();
    expect(screen.getByText("Waiting for releases…")).toBeInTheDocument();
  });

  test("choosing a feed puts it in the URL and reconnects", async () => {
    const user = userEvent.setup();
    mockFeeds(twoFeeds);
    renderPage();

    const picker = await screen.findByLabelText("Feed");
    await user.selectOptions(picker, "no-adult");

    const latest =
      MockEventSource.instances[MockEventSource.instances.length - 1];
    expect(latest.url).toBe(
      "http://localhost:8080/api/v1/announce-stream/no-adult?token=test-token",
    );
    // The stream for the feed left behind must be closed, not left open
    // consuming a slot against the per-user cap.
    expect(MockEventSource.instances[0].closed).toBe(true);
  });

  test("switching feeds clears what the other feed had shown", async () => {
    const user = userEvent.setup();
    mockFeeds(twoFeeds);
    renderPage(["/live?feed=default"]);

    MockEventSource.instances[0].emit("announcement", announcement());
    expect(screen.getByText("Some.Release-GROUP")).toBeInTheDocument();

    const picker = await screen.findByLabelText("Feed");
    await user.selectOptions(picker, "no-adult");

    // The rows on screen belonged to a feed that is no longer being watched;
    // leaving them would misrepresent what this feed carries.
    expect(screen.queryByText("Some.Release-GROUP")).not.toBeInTheDocument();
    expect(screen.getByText("Waiting for releases…")).toBeInTheDocument();
  });
});

describe("LiveReleasesPage access", () => {
  test("says so when the account has no feed access, and opens no stream", async () => {
    // EventSource cannot read a status code, so a refused stream would retry
    // every few seconds forever and read as "reconnecting" to someone who is
    // never getting in. The feed list answers with a real status instead.
    mockFeedsForbidden();
    renderPage();

    expect(
      await screen.findByText(/does not have access to the live feeds/),
    ).toBeInTheDocument();

    // The page opens one stream before the refusal arrives — it cannot know
    // sooner. What matters is that it is closed and not retried, rather than
    // reconnecting every few seconds against a server that will keep saying no.
    expect(MockEventSource.instances.every((source) => source.closed)).toBe(
      true,
    );

    const opened = MockEventSource.instances.length;
    await act(async () => {
      await Promise.resolve();
    });
    expect(MockEventSource.instances).toHaveLength(opened);
  });

  test("shows no status badge or picker when access is refused", async () => {
    mockFeedsForbidden();
    renderPage(["/live?feed=no-adult"]);

    await screen.findByText(/does not have access to the live feeds/);
    expect(screen.queryByLabelText("Feed")).not.toBeInTheDocument();
    expect(screen.queryByText("connecting")).not.toBeInTheDocument();
    expect(screen.queryByText("Waiting for releases…")).not.toBeInTheDocument();
  });
});

describe("LiveReleasesPage access changing mid-session", () => {
  test("a stream dropped because access was revoked ends in the denial message", async () => {
    // The server closes the stream when the privilege is taken away. Without a
    // re-check the page would retry every few seconds forever, reading
    // "reconnecting" to someone who is never getting back in.
    mockFeedsThenForbidden([{ slug: "default", name: "Everything" }]);
    renderPage();

    await waitFor(() => expect(MockEventSource.instances.length).toBe(1));
    MockEventSource.instances[0].fail();

    expect(
      await screen.findByText(/does not have access to the live feeds/),
    ).toBeInTheDocument();
  });

  test("a server error is not read as a refusal", async () => {
    // A 500 means the check could not run, not that the member is refused —
    // showing them a denial would be a lie, and a permanent one.
    vi.stubGlobal(
      "fetch",
      vi.fn(() =>
        Promise.resolve({
          ok: false,
          status: 500,
          json: async () => ({ error: { code: "internal_error" } }),
        }),
      ),
    );
    renderPage();

    await waitFor(() => expect(fetch).toHaveBeenCalled());
    expect(
      screen.queryByText(/does not have access to the live feeds/),
    ).not.toBeInTheDocument();
  });
});
