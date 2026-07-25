import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { LiveReleasesPage } from "@/pages/LiveReleasesPage";

vi.mock("@/features/auth/token", () => ({
  getAccessToken: vi.fn(() => "test-token"),
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

beforeEach(() => {
  MockEventSource.instances = [];
  vi.stubGlobal("EventSource", MockEventSource);
});

afterEach(cleanup);

function renderPage() {
  const result = render(
    <MemoryRouter>
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
});
