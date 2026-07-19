import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { userEvent } from "@testing-library/user-event";
import { afterEach, beforeEach, describe, test, expect, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { NotificationsPage } from "@/pages/NotificationsPage";
import { ToastProvider } from "@/components/toast";

vi.mock("@/features/auth/token", () => ({
  getAccessToken: () => "fake-token",
}));

vi.mock("@/config", () => ({
  getConfig: () => ({ API_URL: "http://localhost:8080", SITE_NAME: "Test" }),
}));

const setNotifUnreadCount = vi.fn();
vi.mock("@/lib/useChat", () => ({
  useChat: () => ({ setNotifUnreadCount }),
}));

const FAKE_PREFERENCES = {
  preferences: [
    { notification_type: "forum_reply", enabled: true },
    { notification_type: "mention", enabled: true },
    { notification_type: "pm_received", enabled: false },
  ],
};

const mockFetch = vi.fn();

afterEach(cleanup);

function jsonResponse(body: unknown) {
  return Promise.resolve({ ok: true, json: () => Promise.resolve(body) });
}

// A collapsed group of 3 topic_reply notifications for the same topic, plus
// one unrelated singleton mention -- exercises both rendering paths.
const GROUPED_RESPONSE = {
  groups: [
    {
      key: "topic_reply:4",
      type: "topic_reply",
      count: 3,
      unread: true,
      last_actors: ["carol", "bob", "alice"],
      latest_created_at: "2026-07-19T12:00:00Z",
      data: { topic_id: 4, topic_title: "Release Discussion" },
      notifications: [
        {
          id: 3,
          type: "topic_reply",
          data: {
            topic_id: 4,
            topic_title: "Release Discussion",
            actor_username: "carol",
          },
          read: false,
          created_at: "2026-07-19T12:00:00Z",
        },
        {
          id: 2,
          type: "topic_reply",
          data: {
            topic_id: 4,
            topic_title: "Release Discussion",
            actor_username: "bob",
          },
          read: false,
          created_at: "2026-07-19T11:00:00Z",
        },
        {
          id: 1,
          type: "topic_reply",
          data: {
            topic_id: 4,
            topic_title: "Release Discussion",
            actor_username: "alice",
          },
          read: true,
          created_at: "2026-07-19T10:00:00Z",
        },
      ],
    },
    {
      key: "single:5",
      type: "mention",
      count: 1,
      unread: true,
      last_actors: ["dave"],
      latest_created_at: "2026-07-19T13:00:00Z",
      data: { actor_username: "dave", context_title: "A thread" },
      notifications: [
        {
          id: 5,
          type: "mention",
          data: { actor_username: "dave", context_title: "A thread" },
          read: false,
          created_at: "2026-07-19T13:00:00Z",
        },
      ],
    },
  ],
  total: 2,
  page: 1,
  per_page: 25,
};

beforeEach(() => {
  vi.clearAllMocks();
  mockFetch.mockImplementation((url: string) => {
    if (url.includes("/digest-preference")) {
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve({ frequency: "off" }),
      });
    }
    if (url.includes("/notifications/preferences")) {
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve(FAKE_PREFERENCES),
      });
    }
    if (url.includes("/notifications/grouped")) {
      return jsonResponse(GROUPED_RESPONSE);
    }
    if (url.includes("/unread-count")) {
      return jsonResponse({ count: 3 });
    }
    if (url.includes("/read-all") || url.includes("/read")) {
      return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
    }
    return jsonResponse({});
  });
  vi.stubGlobal("fetch", mockFetch);
});

function renderNotificationsPage() {
  return render(
    <MemoryRouter>
      <ToastProvider>
        <NotificationsPage />
      </ToastProvider>
    </MemoryRouter>,
  );
}

function renderPage() {
  return render(
    <ToastProvider>
      <MemoryRouter initialEntries={["/notifications"]}>
        <NotificationsPage />
      </MemoryRouter>
    </ToastProvider>,
  );
}

describe("NotificationsPage digest preference", () => {
  test("fetches and displays the current digest frequency when the Preferences tab opens", async () => {
    mockFetch.mockImplementation((url: string) => {
      if (url.includes("/digest-preference")) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ frequency: "weekly" }),
        });
      }
      if (url.includes("/notifications/preferences")) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve(FAKE_PREFERENCES),
        });
      }
      return Promise.resolve({
        ok: true,
        json: () =>
          Promise.resolve({
            notifications: [],
            total: 0,
            page: 1,
            per_page: 25,
          }),
      });
    });

    const user = userEvent.setup();
    renderNotificationsPage();

    await user.click(screen.getByText("Preferences"));

    await waitFor(() => {
      expect(screen.getByLabelText("Email digest")).toBeInTheDocument();
    });
    expect(screen.getByLabelText("Email digest")).toHaveValue("weekly");
  });

  test("defaults to off when no digest preference has been saved", async () => {
    const user = userEvent.setup();
    renderNotificationsPage();

    await user.click(screen.getByText("Preferences"));

    await waitFor(() => {
      expect(screen.getByLabelText("Email digest")).toHaveValue("off");
    });
  });

  test("changing the digest select PUTs the new frequency", async () => {
    const user = userEvent.setup();
    renderNotificationsPage();

    await user.click(screen.getByText("Preferences"));
    await waitFor(() => {
      expect(screen.getByLabelText("Email digest")).toBeInTheDocument();
    });

    await user.selectOptions(screen.getByLabelText("Email digest"), "daily");

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining("/api/v1/notifications/digest-preference"),
        expect.objectContaining({
          method: "PUT",
          body: JSON.stringify({ frequency: "daily" }),
        }),
      );
    });
    expect(screen.getByLabelText("Email digest")).toHaveValue("daily");
  });

  test("reverts the select and shows an error toast when the update fails", async () => {
    mockFetch.mockImplementation((url: string, init?: RequestInit) => {
      if (url.includes("/digest-preference") && init?.method === "PUT") {
        return Promise.resolve({ ok: false, json: () => Promise.resolve({}) });
      }
      if (url.includes("/digest-preference")) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ frequency: "off" }),
        });
      }
      if (url.includes("/notifications/preferences")) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve(FAKE_PREFERENCES),
        });
      }
      return Promise.resolve({
        ok: true,
        json: () =>
          Promise.resolve({
            notifications: [],
            total: 0,
            page: 1,
            per_page: 25,
          }),
      });
    });

    const user = userEvent.setup();
    renderNotificationsPage();

    await user.click(screen.getByText("Preferences"));
    await waitFor(() => {
      expect(screen.getByLabelText("Email digest")).toHaveValue("off");
    });

    await user.selectOptions(screen.getByLabelText("Email digest"), "weekly");

    await waitFor(() => {
      expect(
        screen.getByText("Failed to update digest preference"),
      ).toBeInTheDocument();
    });
    expect(screen.getByLabelText("Email digest")).toHaveValue("off");
  });

  test("still renders the per-type preference toggles alongside the digest control", async () => {
    const user = userEvent.setup();
    renderNotificationsPage();

    await user.click(screen.getByText("Preferences"));

    await waitFor(() => {
      expect(screen.getByText("Mention")).toBeInTheDocument();
    });
    expect(screen.getByText("Private Message")).toBeInTheDocument();
    expect(screen.getByLabelText("Email digest")).toBeInTheDocument();
  });
});

describe("NotificationsPage grouping (BE-9.14)", () => {
  test("fetches from the grouped endpoint, not the flat one", async () => {
    renderPage();
    await waitFor(() =>
      expect(
        mockFetch.mock.calls.some(([url]) =>
          String(url).includes("/api/v1/notifications/grouped"),
        ),
      ).toBe(true),
    );
  });

  test("shows the collapsed count and last actors for a topic_reply group", async () => {
    renderPage();
    expect(
      await screen.findByText(
        '3 new replies in "Release Discussion" from carol, bob, and alice',
      ),
    ).toBeInTheDocument();
  });

  test("a singleton group renders like a normal notification", async () => {
    renderPage();
    expect(
      await screen.findByText('dave mentioned you in "A thread"'),
    ).toBeInTheDocument();
  });

  test("individual notifications are hidden until the group is expanded", async () => {
    renderPage();
    await screen.findByText(
      '3 new replies in "Release Discussion" from carol, bob, and alice',
    );

    expect(screen.queryByText(/carol posted in/)).not.toBeInTheDocument();

    const user = userEvent.setup();
    await user.click(
      screen.getByRole("button", { name: /show individual replies/i }),
    );

    expect(await screen.findByText(/carol posted in/)).toBeInTheDocument();
    expect(screen.getByText(/bob posted in/)).toBeInTheDocument();
    expect(screen.getByText(/alice posted in/)).toBeInTheDocument();

    // Collapses back on a second click.
    await user.click(
      screen.getByRole("button", { name: /hide individual replies/i }),
    );
    expect(screen.queryByText(/carol posted in/)).not.toBeInTheDocument();
  });

  test("marking all read on a group PUTs each still-unread notification", async () => {
    renderPage();
    await screen.findByText(
      '3 new replies in "Release Discussion" from carol, bob, and alice',
    );

    // Two "Mark all read" controls exist: the page header (marks
    // everything) and the group's own (marks just that group). The group's
    // is the one rendered inside the collapsed entry.
    const user = userEvent.setup();
    const groupMarkAllRead = screen
      .getAllByRole("button", { name: /mark all read/i })
      .find((btn) => btn.closest(".notifs-group"));
    expect(groupMarkAllRead).toBeDefined();
    await user.click(groupMarkAllRead!);

    await waitFor(() => {
      const readUrls = mockFetch.mock.calls
        .map(([url]) => String(url))
        .filter((url) => /\/notifications\/\d+\/read$/.test(url));
      // Only notifications 2 and 3 were unread in the group (1 was already read).
      expect(readUrls.sort()).toEqual(
        [
          "http://localhost:8080/api/v1/notifications/2/read",
          "http://localhost:8080/api/v1/notifications/3/read",
        ].sort(),
      );
    });
  });

  // Regression test: handleMarkGroupRead used to call handleMarkRead per
  // notification, and each one independently refetched /unread-count,
  // racing N GETs against each other for a single group action. It should
  // now refetch exactly once, after every PUT in the group has settled.
  test("marking a group read refetches the unread count exactly once", async () => {
    renderPage();
    await screen.findByText(
      '3 new replies in "Release Discussion" from carol, bob, and alice',
    );

    expect(
      mockFetch.mock.calls.filter(([url]) =>
        String(url).includes("/unread-count"),
      ),
    ).toHaveLength(0);

    const user = userEvent.setup();
    const groupMarkAllRead = screen
      .getAllByRole("button", { name: /mark all read/i })
      .find((btn) => btn.closest(".notifs-group"));
    await user.click(groupMarkAllRead!);

    await waitFor(() => {
      expect(setNotifUnreadCount).toHaveBeenCalledTimes(1);
    });
    expect(
      mockFetch.mock.calls.filter(([url]) =>
        String(url).includes("/unread-count"),
      ),
    ).toHaveLength(1);
  });
});

describe("NotificationsPage page clamping", () => {
  // Marking a whole group read can wipe out an entire page in one click.
  // If the page the user was on no longer exists, the view should snap
  // back to the last valid page instead of rendering an empty page while
  // earlier pages still have content.
  test("snaps back to the last valid page when it no longer exists", async () => {
    let page2Requested = false;
    mockFetch.mockImplementation((url: string) => {
      if (url.includes("/notifications/grouped")) {
        const params = new URL(url).searchParams;
        if (params.get("page") === "2") {
          page2Requested = true;
          // Page 2 no longer exists (e.g. its only group was just marked
          // read entirely from another tab) -- total now fits on page 1.
          return jsonResponse({ groups: [], total: 1, page: 2, per_page: 25 });
        }
        // 30 singleton groups -> 2 pages at PER_PAGE=25.
        const groups = Array.from({ length: 25 }, (_, i) => ({
          key: `single:${i}`,
          type: "mention",
          count: 1,
          unread: false,
          last_actors: [],
          latest_created_at: "2026-07-19T12:00:00Z",
          data: { actor_username: "alice" },
          notifications: [
            {
              id: i,
              type: "mention",
              data: { actor_username: "alice" },
              read: true,
              created_at: "2026-07-19T12:00:00Z",
            },
          ],
        }));
        return jsonResponse({ groups, total: 30, page: 1, per_page: 25 });
      }
      if (url.includes("/unread-count")) return jsonResponse({ count: 0 });
      return jsonResponse({});
    });
    vi.stubGlobal("fetch", mockFetch);

    renderPage();
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: /next/i }));

    await waitFor(() => expect(page2Requested).toBe(true));

    // The page-2 response reported total=1 (fits entirely on page 1), so
    // the view should have snapped back to page 1 and refetched it rather
    // than rendering the empty page-2 response.
    await waitFor(() => {
      const page1Requests = mockFetch.mock.calls.filter(([url]) => {
        const u = String(url);
        return (
          u.includes("/notifications/grouped") &&
          new URL(u).searchParams.get("page") === "1"
        );
      });
      // Once on mount, once after snapping back from the vanished page 2.
      expect(page1Requests.length).toBeGreaterThanOrEqual(2);
    });
  });
});
