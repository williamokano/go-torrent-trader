import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { userEvent } from "@testing-library/user-event";
import { afterEach, beforeEach, describe, test, expect, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { MessagesPage } from "@/pages/MessagesPage";

vi.mock("@/features/auth/token", () => ({
  getAccessToken: () => "fake-token",
}));

vi.mock("@/config", () => ({
  getConfig: () => ({ API_URL: "http://localhost:8080", SITE_NAME: "Test" }),
}));

vi.mock("@/lib/useChat", () => ({
  useChat: () => ({
    pmUnreadCount: 0,
    setPmUnreadCount: vi.fn(),
    messages: [],
    connected: false,
    isStaff: false,
    muted: false,
    muteExpiresAt: null,
    mainChatVisible: false,
    setMainChatVisible: vi.fn(),
    sendMessage: vi.fn(),
    deleteMessage: vi.fn(),
    deleteUserMessages: vi.fn(),
    muteUser: vi.fn(),
    unmuteUser: vi.fn(),
    loadMore: vi.fn(),
  }),
}));

const FAKE_INBOX = [
  {
    id: 1,
    sender_id: 2,
    sender_username: "alice",
    receiver_id: 1,
    receiver_username: "testuser",
    subject: "Hello there",
    body: "How are you?",
    is_read: false,
    created_at: "2026-03-08T10:00:00Z",
  },
  {
    id: 2,
    sender_id: 3,
    sender_username: "bob",
    receiver_id: 1,
    receiver_username: "testuser",
    subject: "Meeting tomorrow",
    body: "Let us discuss.",
    is_read: true,
    created_at: "2026-03-07T10:00:00Z",
  },
];

const FAKE_OUTBOX = [
  {
    id: 3,
    sender_id: 1,
    sender_username: "testuser",
    receiver_id: 2,
    receiver_username: "alice",
    subject: "Reply: Hello",
    body: "I am fine!",
    is_read: true,
    created_at: "2026-03-08T11:00:00Z",
  },
];

const mockFetch = vi.fn();

afterEach(cleanup);

beforeEach(() => {
  vi.clearAllMocks();
  mockFetch.mockImplementation((url: string) => {
    if (url.includes("/unread-count")) {
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve({ unread_count: 1 }),
      });
    }
    if (url.includes("/inbox")) {
      return Promise.resolve({
        ok: true,
        json: () =>
          Promise.resolve({
            messages: FAKE_INBOX,
            total: 2,
            page: 1,
            per_page: 25,
          }),
      });
    }
    if (url.includes("/outbox")) {
      return Promise.resolve({
        ok: true,
        json: () =>
          Promise.resolve({
            messages: FAKE_OUTBOX,
            total: 1,
            page: 1,
            per_page: 25,
          }),
      });
    }
    return Promise.resolve({
      ok: true,
      json: () => Promise.resolve({}),
    });
  });
  vi.stubGlobal("fetch", mockFetch);
});

function renderMessagesPage() {
  return render(
    <MemoryRouter initialEntries={["/messages"]}>
      <MessagesPage />
    </MemoryRouter>,
  );
}

describe("MessagesPage markdown", () => {
  function renderDetail(body: string, mentionedUsernames: string[] = []) {
    mockFetch.mockImplementation((url: string) => {
      if (url.includes("/unread-count")) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ unread_count: 0 }),
        });
      }
      if (/\/api\/v1\/messages\/1$/.test(url)) {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              message: {
                ...FAKE_INBOX[0],
                body,
                mentioned_usernames: mentionedUsernames,
              },
            }),
        });
      }
      return Promise.resolve({
        ok: true,
        json: () =>
          Promise.resolve({
            messages: FAKE_INBOX,
            total: 2,
            page: 1,
            per_page: 25,
          }),
      });
    });

    return render(
      <MemoryRouter initialEntries={["/messages?msg=1"]}>
        <MessagesPage />
      </MemoryRouter>,
    );
  }

  test("renders the message body as markdown", async () => {
    const { container } = renderDetail("**urgent** and !!secret!!");

    await waitFor(() => {
      expect(container.querySelector("strong")?.textContent).toBe("urgent");
    });
    expect(container.querySelector("details")).toBeInTheDocument();
  });

  test("sanitizes XSS payloads in the message body", async () => {
    const { container } = renderDetail(
      '<script>alert("xss")</script><img src=x onerror="alert(1)">',
    );

    await waitFor(() => {
      expect(screen.getByText("Hello there")).toBeInTheDocument();
    });
    expect(container.querySelector("script")).not.toBeInTheDocument();
    expect(container.innerHTML).not.toContain("onerror");
    expect(container.innerHTML).not.toContain("alert");
  });

  test("links a resolved @mention in the message body to the mentioned user's profile", async () => {
    const { container } = renderDetail("hey @alice, check this", ["alice"]);

    await waitFor(() => {
      expect(container.querySelector("a.mention")).toBeInTheDocument();
    });
    const link = container.querySelector("a.mention");
    expect(link?.getAttribute("href")).toBe("/user/alice");
    expect(link?.textContent).toBe("@alice");
  });

  test("does not link an @mention the backend never resolved", async () => {
    const { container } = renderDetail("hey @notauser", []);

    await waitFor(() => {
      expect(screen.getByText("Hello there")).toBeInTheDocument();
    });
    expect(container.querySelector("a.mention")).not.toBeInTheDocument();
    expect(container.textContent).toContain("@notauser");
  });

  test("composes messages with the markdown editor", async () => {
    const user = userEvent.setup();
    renderMessagesPage();
    await waitFor(() => {
      expect(screen.getByText("Hello there")).toBeInTheDocument();
    });

    await user.click(screen.getByText("Compose"));

    expect(screen.getByLabelText("Message")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Bold" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Preview" })).toBeInTheDocument();
  });
});

describe("MessagesPage", () => {
  test("renders page title", () => {
    renderMessagesPage();
    expect(screen.getByText("Messages")).toBeInTheDocument();
  });

  test("renders tabs", () => {
    renderMessagesPage();
    expect(screen.getByText("Inbox")).toBeInTheDocument();
    expect(screen.getByText("Outbox")).toBeInTheDocument();
    expect(screen.getByText("Compose")).toBeInTheDocument();
  });

  test("shows loading state initially", () => {
    mockFetch.mockImplementation((url: string) => {
      if (url.includes("/unread-count")) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ unread_count: 0 }),
        });
      }
      return new Promise(() => {});
    });
    renderMessagesPage();
    expect(screen.getByText("Loading messages...")).toBeInTheDocument();
  });

  test("renders inbox messages after loading", async () => {
    renderMessagesPage();
    await waitFor(() => {
      expect(screen.getByText("Hello there")).toBeInTheDocument();
    });
    expect(screen.getByText("Meeting tomorrow")).toBeInTheDocument();
    expect(screen.getByText("alice")).toBeInTheDocument();
    expect(screen.getByText("bob")).toBeInTheDocument();
  });

  test("shows table headers for inbox", async () => {
    renderMessagesPage();
    await waitFor(() => {
      expect(screen.getByText("Hello there")).toBeInTheDocument();
    });
    expect(screen.getByText("From")).toBeInTheDocument();
    expect(screen.getByText("Subject")).toBeInTheDocument();
    expect(screen.getByText("Date")).toBeInTheDocument();
  });

  test("switches to outbox tab", async () => {
    const user = userEvent.setup();
    renderMessagesPage();
    await waitFor(() => {
      expect(screen.getByText("Hello there")).toBeInTheDocument();
    });

    await user.click(screen.getByText("Outbox"));
    await waitFor(() => {
      expect(screen.getByText("Reply: Hello")).toBeInTheDocument();
    });
  });

  test("switches to compose tab", async () => {
    const user = userEvent.setup();
    renderMessagesPage();
    await waitFor(() => {
      expect(screen.getByText("Hello there")).toBeInTheDocument();
    });

    await user.click(screen.getByText("Compose"));
    expect(screen.getByLabelText("To")).toBeInTheDocument();
    expect(screen.getByLabelText("Subject")).toBeInTheDocument();
    expect(screen.getByLabelText("Message")).toBeInTheDocument();
    expect(screen.getByText("Send Message")).toBeInTheDocument();
  });

  test("shows empty inbox state", async () => {
    mockFetch.mockImplementation((url: string) => {
      if (url.includes("/unread-count")) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ unread_count: 0 }),
        });
      }
      return Promise.resolve({
        ok: true,
        json: () =>
          Promise.resolve({ messages: [], total: 0, page: 1, per_page: 25 }),
      });
    });
    renderMessagesPage();
    await waitFor(() => {
      expect(screen.getByText("Your inbox is empty.")).toBeInTheDocument();
    });
  });

  test("shows error state on API failure", async () => {
    mockFetch.mockImplementation((url: string) => {
      if (url.includes("/unread-count")) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ unread_count: 0 }),
        });
      }
      return Promise.resolve({
        ok: false,
        json: () => Promise.resolve({ error: { message: "Server error" } }),
      });
    });
    renderMessagesPage();
    await waitFor(() => {
      expect(screen.getByText("Server error")).toBeInTheDocument();
    });
  });

  test("shows unread badge", async () => {
    renderMessagesPage();
    await waitFor(() => {
      expect(screen.getByText("1")).toBeInTheDocument();
    });
  });

  test("passes authorization header to fetch", async () => {
    renderMessagesPage();
    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining("/api/v1/messages/inbox"),
        expect.objectContaining({
          headers: { Authorization: "Bearer fake-token" },
        }),
      );
    });
  });
});

describe("MessagesPage drafts & templates (BE-7.2)", () => {
  const FAKE_DRAFT = {
    id: 1,
    kind: "draft",
    receiver_id: 2,
    receiver_username: "alice",
    subject: "unfinished thought",
    body: "still writing this...",
    version: 3,
    created_at: "2026-03-08T10:00:00Z",
    updated_at: "2026-03-08T10:05:00Z",
  };

  const FAKE_TEMPLATE = {
    id: 5,
    kind: "template",
    subject: "Welcome",
    body: "Thanks for joining!",
    version: 1,
    created_at: "2026-03-08T09:00:00Z",
    updated_at: "2026-03-08T09:00:00Z",
  };

  function baseFetchMock(url: string, init?: RequestInit) {
    const method = init?.method ?? "GET";

    if (url.includes("/unread-count")) {
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve({ unread_count: 0 }),
      });
    }
    if (url.includes("/inbox")) {
      return Promise.resolve({
        ok: true,
        json: () =>
          Promise.resolve({
            messages: FAKE_INBOX,
            total: 2,
            page: 1,
            per_page: 25,
          }),
      });
    }
    if (/\/api\/v1\/messages\/drafts\/1$/.test(url) && method === "GET") {
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve({ draft: FAKE_DRAFT }),
      });
    }
    if (/\/api\/v1\/messages\/templates\/5$/.test(url) && method === "GET") {
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve({ template: FAKE_TEMPLATE }),
      });
    }
    if (url.includes("/api/v1/messages/drafts") && method === "GET") {
      return Promise.resolve({
        ok: true,
        json: () =>
          Promise.resolve({
            drafts: [FAKE_DRAFT],
            total: 1,
            page: 1,
            per_page: 25,
          }),
      });
    }
    if (url.includes("/api/v1/messages/templates") && method === "GET") {
      return Promise.resolve({
        ok: true,
        json: () =>
          Promise.resolve({
            templates: [FAKE_TEMPLATE],
            total: 1,
            page: 1,
            per_page: 25,
          }),
      });
    }
    return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
  }

  beforeEach(() => {
    mockFetch.mockImplementation(baseFetchMock);
  });

  test("renders Drafts and Templates tabs", () => {
    renderMessagesPage();
    expect(screen.getByText("Drafts")).toBeInTheDocument();
    expect(screen.getByText("Templates")).toBeInTheDocument();
  });

  test("lists saved drafts with recipient and subject", async () => {
    const user = userEvent.setup();
    renderMessagesPage();

    await user.click(screen.getByText("Drafts"));
    await waitFor(() => {
      expect(screen.getByText("unfinished thought")).toBeInTheDocument();
    });
    expect(screen.getByText("alice")).toBeInTheDocument();
  });

  test("shows empty state when there are no drafts", async () => {
    mockFetch.mockImplementation((url: string, init?: RequestInit) => {
      if (url.includes("/unread-count")) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ unread_count: 0 }),
        });
      }
      if (url.includes("/api/v1/messages/drafts") && !init?.method) {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({ drafts: [], total: 0, page: 1, per_page: 25 }),
        });
      }
      return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
    });
    const user = userEvent.setup();
    renderMessagesPage();

    await user.click(screen.getByText("Drafts"));
    await waitFor(() => {
      expect(screen.getByText("No saved drafts.")).toBeInTheDocument();
    });
  });

  test("lists saved templates", async () => {
    const user = userEvent.setup();
    renderMessagesPage();

    await user.click(screen.getByText("Templates"));
    await waitFor(() => {
      expect(screen.getByText("Welcome")).toBeInTheDocument();
    });
  });

  test("loading a draft populates the compose form and switches tabs", async () => {
    const user = userEvent.setup();
    renderMessagesPage();

    await user.click(screen.getByText("Drafts"));
    await waitFor(() => {
      expect(screen.getByText("unfinished thought")).toBeInTheDocument();
    });

    await user.click(screen.getByText("unfinished thought"));

    await waitFor(() => {
      expect(screen.getByLabelText("To")).toHaveValue("alice");
    });
    expect(screen.getByLabelText("Subject")).toHaveValue("unfinished thought");
    expect(screen.getByText("Update Draft")).toBeInTheDocument();
  });

  test("using a template populates the compose form without a recipient", async () => {
    const user = userEvent.setup();
    renderMessagesPage();

    await user.click(screen.getByText("Templates"));
    await waitFor(() => {
      expect(screen.getByText("Welcome")).toBeInTheDocument();
    });

    await user.click(screen.getByText("Welcome"));

    await waitFor(() => {
      expect(screen.getByLabelText("Subject")).toHaveValue("Welcome");
    });
    expect(screen.getByLabelText("To")).toHaveValue("");
    expect(screen.getByText("Update Template")).toBeInTheDocument();
  });

  test("deleting a draft calls the drafts delete endpoint", async () => {
    const user = userEvent.setup();
    renderMessagesPage();

    await user.click(screen.getByText("Drafts"));
    await waitFor(() => {
      expect(screen.getByText("unfinished thought")).toBeInTheDocument();
    });

    await user.click(screen.getByText("Delete"));

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining("/api/v1/messages/drafts/1"),
        expect.objectContaining({ method: "DELETE" }),
      );
    });
  });

  test("Save Draft is rejected client-side when the compose form is entirely empty", async () => {
    const user = userEvent.setup();
    renderMessagesPage();
    await waitFor(() => {
      expect(screen.getByText("Hello there")).toBeInTheDocument();
    });

    await user.click(screen.getByText("Compose"));
    await user.click(screen.getByText("Save Draft"));

    await waitFor(() => {
      expect(
        screen.getByText("Nothing to save yet — write something first."),
      ).toBeInTheDocument();
    });
    expect(mockFetch).not.toHaveBeenCalledWith(
      expect.stringContaining("/api/v1/messages/drafts"),
      expect.objectContaining({ method: "POST" }),
    );
  });

  test("Save Draft creates a draft from the compose form", async () => {
    mockFetch.mockImplementation((url: string, init?: RequestInit) => {
      const method = init?.method ?? "GET";
      if (url.includes("/unread-count")) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ unread_count: 0 }),
        });
      }
      if (url.includes("/inbox")) {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              messages: FAKE_INBOX,
              total: 2,
              page: 1,
              per_page: 25,
            }),
        });
      }
      if (url.endsWith("/api/v1/messages/drafts") && method === "POST") {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ draft: { ...FAKE_DRAFT, id: 9 } }),
        });
      }
      return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
    });

    const user = userEvent.setup();
    renderMessagesPage();
    await waitFor(() => {
      expect(screen.getByText("Hello there")).toBeInTheDocument();
    });

    await user.click(screen.getByText("Compose"));
    await user.type(screen.getByLabelText("Subject"), "Draft subject");
    await user.click(screen.getByText("Save Draft"));

    await waitFor(() => {
      expect(screen.getByText("Draft saved.")).toBeInTheDocument();
    });
    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining("/api/v1/messages/drafts"),
      expect.objectContaining({ method: "POST" }),
    );
  });

  test("Save as Template is disabled until the body has content", async () => {
    const user = userEvent.setup();
    renderMessagesPage();
    await waitFor(() => {
      expect(screen.getByText("Hello there")).toBeInTheDocument();
    });

    await user.click(screen.getByText("Compose"));
    expect(screen.getByText("Save as Template")).toBeDisabled();
  });

  test("changing the recipient after replying drops the reply's parent_id from a saved draft", async () => {
    let capturedBody: string | undefined;
    mockFetch.mockImplementation((url: string, init?: RequestInit) => {
      const method = init?.method ?? "GET";
      if (url.includes("/unread-count")) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ unread_count: 0 }),
        });
      }
      if (url.includes("/inbox")) {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              messages: FAKE_INBOX,
              total: 2,
              page: 1,
              per_page: 25,
            }),
        });
      }
      if (/\/api\/v1\/messages\/1$/.test(url) && method === "GET") {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ message: FAKE_INBOX[0] }),
        });
      }
      if (url.includes("/api/v1/users?search=")) {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({ users: [{ id: 9, username: "carol" }] }),
        });
      }
      if (url.endsWith("/api/v1/messages/drafts") && method === "POST") {
        capturedBody = init?.body as string;
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ draft: { id: 1 } }),
        });
      }
      return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
    });

    const user = userEvent.setup();
    renderMessagesPage();
    await waitFor(() => {
      expect(screen.getByText("Hello there")).toBeInTheDocument();
    });

    await user.click(screen.getByText("Hello there"));
    await waitFor(() => {
      expect(screen.getByText("Reply")).toBeInTheDocument();
    });
    await user.click(screen.getByText("Reply"));

    await waitFor(() => {
      expect(screen.getByLabelText("To")).toHaveValue("alice");
    });

    // Change the recipient away from alice, the original reply target — the
    // parent_id from that reply must not silently carry over to whoever
    // ends up selected instead.
    await user.clear(screen.getByLabelText("To"));
    await user.type(screen.getByLabelText("To"), "carol");
    await waitFor(() => {
      expect(screen.getByText("carol")).toBeInTheDocument();
    });
    await user.click(screen.getByText("carol"));

    await user.click(screen.getByText("Save Draft"));

    await waitFor(() => {
      expect(capturedBody).toBeDefined();
    });
    const parsed = JSON.parse(capturedBody as string);
    expect(parsed.parent_id).toBeUndefined();
    expect(parsed.receiver_id).toBe(9);
  });

  test("sending a message that was loaded from a draft deletes that draft", async () => {
    const deleteUrls: string[] = [];
    const SAVED_DRAFT = {
      id: 1,
      kind: "draft",
      receiver_id: 2,
      receiver_username: "alice",
      subject: "Hi",
      body: "Body text",
      created_at: "2026-03-08T10:00:00Z",
      updated_at: "2026-03-08T10:00:00Z",
    };
    mockFetch.mockImplementation((url: string, init?: RequestInit) => {
      const method = init?.method ?? "GET";
      if (url.includes("/unread-count")) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ unread_count: 0 }),
        });
      }
      if (url.includes("/inbox")) {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              messages: FAKE_INBOX,
              total: 2,
              page: 1,
              per_page: 25,
            }),
        });
      }
      if (/\/api\/v1\/messages\/drafts\/1$/.test(url) && method === "GET") {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ draft: SAVED_DRAFT }),
        });
      }
      if (url.includes("/api/v1/messages/drafts") && method === "GET") {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              drafts: [SAVED_DRAFT],
              total: 1,
              page: 1,
              per_page: 25,
            }),
        });
      }
      if (url.endsWith("/api/v1/messages") && method === "POST") {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ message: { id: 99 } }),
        });
      }
      if (/\/api\/v1\/messages\/drafts\/1$/.test(url) && method === "DELETE") {
        deleteUrls.push(url);
        return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
      }
      return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
    });

    const user = userEvent.setup();
    renderMessagesPage();
    await waitFor(() => {
      expect(screen.getByText("Hello there")).toBeInTheDocument();
    });

    await user.click(screen.getByText("Drafts"));
    await waitFor(() => {
      expect(screen.getByText("Hi")).toBeInTheDocument();
    });
    await user.click(screen.getByText("Hi"));

    await waitFor(() => {
      expect(screen.getByLabelText("Subject")).toHaveValue("Hi");
    });

    await user.click(screen.getByText("Send Message"));

    await waitFor(() => {
      expect(deleteUrls).toEqual([
        expect.stringContaining("/api/v1/messages/drafts/1"),
      ]);
    });
  });

  test("Save Draft surfaces an error instead of crashing on a malformed response", async () => {
    mockFetch.mockImplementation((url: string, init?: RequestInit) => {
      const method = init?.method ?? "GET";
      if (url.includes("/unread-count")) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ unread_count: 0 }),
        });
      }
      if (url.includes("/inbox")) {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              messages: FAKE_INBOX,
              total: 2,
              page: 1,
              per_page: 25,
            }),
        });
      }
      if (url.endsWith("/api/v1/messages/drafts") && method === "POST") {
        // ok:true but no `draft` key — a malformed/unexpected success body.
        return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
      }
      return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
    });

    const user = userEvent.setup();
    renderMessagesPage();
    await waitFor(() => {
      expect(screen.getByText("Hello there")).toBeInTheDocument();
    });

    await user.click(screen.getByText("Compose"));
    await user.type(screen.getByLabelText("Subject"), "Draft subject");
    await user.click(screen.getByText("Save Draft"));

    await waitFor(() => {
      expect(screen.getByText("Failed to save draft")).toBeInTheDocument();
    });
    expect(screen.queryByText("Draft saved.")).not.toBeInTheDocument();
  });

  // --- optimistic concurrency (BE-7.5) --------------------------------------

  test("updating a loaded draft sends the version it was loaded with", async () => {
    let capturedBody: string | undefined;
    mockFetch.mockImplementation((url: string, init?: RequestInit) => {
      const method = init?.method ?? "GET";
      if (url.includes("/unread-count")) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ unread_count: 0 }),
        });
      }
      if (url.includes("/inbox")) {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              messages: FAKE_INBOX,
              total: 2,
              page: 1,
              per_page: 25,
            }),
        });
      }
      if (/\/api\/v1\/messages\/drafts\/1$/.test(url) && method === "GET") {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ draft: FAKE_DRAFT }),
        });
      }
      if (/\/api\/v1\/messages\/drafts\/1$/.test(url) && method === "PUT") {
        capturedBody = init?.body as string;
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              draft: { ...FAKE_DRAFT, version: FAKE_DRAFT.version + 1 },
            }),
        });
      }
      if (url.includes("/api/v1/messages/drafts") && method === "GET") {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              drafts: [FAKE_DRAFT],
              total: 1,
              page: 1,
              per_page: 25,
            }),
        });
      }
      return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
    });

    const user = userEvent.setup();
    renderMessagesPage();
    await waitFor(() => {
      expect(screen.getByText("Hello there")).toBeInTheDocument();
    });

    await user.click(screen.getByText("Drafts"));
    await waitFor(() => {
      expect(screen.getByText("unfinished thought")).toBeInTheDocument();
    });
    await user.click(screen.getByText("unfinished thought"));
    await waitFor(() => {
      expect(screen.getByText("Update Draft")).toBeInTheDocument();
    });

    await user.click(screen.getByText("Update Draft"));

    await waitFor(() => {
      expect(capturedBody).toBeDefined();
    });
    const parsed = JSON.parse(capturedBody as string);
    expect(parsed.version).toBe(FAKE_DRAFT.version);
  });

  test("a stale Save Draft is rejected with a 409 and offers to load the latest version, without discarding local edits", async () => {
    const CURRENT_ON_SERVER = {
      ...FAKE_DRAFT,
      subject: "someone else's newer subject",
      body: "someone else's newer edit",
      version: FAKE_DRAFT.version + 1,
    };
    mockFetch.mockImplementation((url: string, init?: RequestInit) => {
      const method = init?.method ?? "GET";
      if (url.includes("/unread-count")) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ unread_count: 0 }),
        });
      }
      if (url.includes("/inbox")) {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              messages: FAKE_INBOX,
              total: 2,
              page: 1,
              per_page: 25,
            }),
        });
      }
      if (/\/api\/v1\/messages\/drafts\/1$/.test(url) && method === "GET") {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ draft: FAKE_DRAFT }),
        });
      }
      if (/\/api\/v1\/messages\/drafts\/1$/.test(url) && method === "PUT") {
        return Promise.resolve({
          ok: false,
          status: 409,
          json: () =>
            Promise.resolve({
              error: {
                code: "conflict",
                message: "this was changed elsewhere",
              },
              draft: CURRENT_ON_SERVER,
            }),
        });
      }
      if (url.includes("/api/v1/messages/drafts") && method === "GET") {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              drafts: [FAKE_DRAFT],
              total: 1,
              page: 1,
              per_page: 25,
            }),
        });
      }
      return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
    });

    const user = userEvent.setup();
    renderMessagesPage();
    await waitFor(() => {
      expect(screen.getByText("Hello there")).toBeInTheDocument();
    });

    await user.click(screen.getByText("Drafts"));
    await waitFor(() => {
      expect(screen.getByText("unfinished thought")).toBeInTheDocument();
    });
    await user.click(screen.getByText("unfinished thought"));
    await waitFor(() => {
      expect(screen.getByText("Update Draft")).toBeInTheDocument();
    });

    // A local edit made before saving must survive the conflict — a 409
    // must not silently discard whatever the user was in the middle of
    // writing.
    await user.clear(screen.getByLabelText("Subject"));
    await user.type(screen.getByLabelText("Subject"), "my local edit");

    await user.click(screen.getByText("Update Draft"));

    await waitFor(() => {
      expect(screen.getByText(/changed elsewhere/)).toBeInTheDocument();
    });
    expect(screen.getByText("Load latest version")).toBeInTheDocument();
    expect(screen.getByLabelText("Subject")).toHaveValue("my local edit");
  });

  // Regression test for a bug caught in review: the error banner and its
  // "Load latest version" action used to be two independently-set pieces of
  // state, so a later, unrelated error (one with nothing to do with the
  // conflict) could leave the stale "Load latest version" button attached to
  // it — clicking it would then discard the wrong content into the compose
  // form. They're now a single atomic errorBanner, so any new error clears
  // out a previous conflict along with it.
  test("an unrelated error after a 409 conflict clears the stale Load latest version action", async () => {
    const CURRENT_ON_SERVER = {
      ...FAKE_DRAFT,
      subject: "someone else's newer subject",
      body: "someone else's newer edit",
      version: FAKE_DRAFT.version + 1,
    };
    mockFetch.mockImplementation((url: string, init?: RequestInit) => {
      const method = init?.method ?? "GET";
      if (url.includes("/unread-count")) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ unread_count: 0 }),
        });
      }
      if (url.includes("/inbox")) {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              messages: FAKE_INBOX,
              total: 2,
              page: 1,
              per_page: 25,
            }),
        });
      }
      if (/\/api\/v1\/messages\/drafts\/1$/.test(url) && method === "GET") {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ draft: FAKE_DRAFT }),
        });
      }
      if (/\/api\/v1\/messages\/drafts\/1$/.test(url) && method === "PUT") {
        return Promise.resolve({
          ok: false,
          status: 409,
          json: () =>
            Promise.resolve({
              error: {
                code: "conflict",
                message: "this was changed elsewhere",
              },
              draft: CURRENT_ON_SERVER,
            }),
        });
      }
      if (url.includes("/api/v1/messages/drafts") && method === "GET") {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              drafts: [FAKE_DRAFT],
              total: 1,
              page: 1,
              per_page: 25,
            }),
        });
      }
      return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
    });

    const user = userEvent.setup();
    renderMessagesPage();
    await waitFor(() => {
      expect(screen.getByText("Hello there")).toBeInTheDocument();
    });

    await user.click(screen.getByText("Drafts"));
    await waitFor(() => {
      expect(screen.getByText("unfinished thought")).toBeInTheDocument();
    });
    await user.click(screen.getByText("unfinished thought"));
    await waitFor(() => {
      expect(screen.getByText("Update Draft")).toBeInTheDocument();
    });

    // First, hit the 409 and confirm the conflict banner shows up.
    await user.click(screen.getByText("Update Draft"));
    await waitFor(() => {
      expect(screen.getByText("Load latest version")).toBeInTheDocument();
    });

    // Now clear the whole form — including the recipient, which also nulls
    // out composeReceiverId (see the "To" field's onChange) — and trigger a
    // wholly unrelated, purely client-side error: the compose form has
    // nothing to save at all, so this never even reaches the network.
    await user.clear(screen.getByLabelText("To"));
    await user.clear(screen.getByLabelText("Subject"));
    await user.clear(screen.getByLabelText("Message"));
    await user.click(screen.getByText("Update Draft"));

    await waitFor(() => {
      expect(
        screen.getByText("Nothing to save yet — write something first."),
      ).toBeInTheDocument();
    });
    // The stale conflict action must not survive into this new, unrelated
    // error — otherwise clicking it would silently load the old conflict's
    // draft over whatever the user is doing now.
    expect(screen.queryByText("Load latest version")).not.toBeInTheDocument();
  });

  test("Load latest version replaces the compose form and lets a follow-up save go through", async () => {
    const CURRENT_ON_SERVER = {
      ...FAKE_DRAFT,
      subject: "someone else's newer subject",
      body: "someone else's newer edit",
      version: FAKE_DRAFT.version + 1,
    };
    let putCount = 0;
    let lastPutVersion: number | undefined;
    mockFetch.mockImplementation((url: string, init?: RequestInit) => {
      const method = init?.method ?? "GET";
      if (url.includes("/unread-count")) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ unread_count: 0 }),
        });
      }
      if (url.includes("/inbox")) {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              messages: FAKE_INBOX,
              total: 2,
              page: 1,
              per_page: 25,
            }),
        });
      }
      if (/\/api\/v1\/messages\/drafts\/1$/.test(url) && method === "GET") {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ draft: FAKE_DRAFT }),
        });
      }
      if (/\/api\/v1\/messages\/drafts\/1$/.test(url) && method === "PUT") {
        putCount += 1;
        const parsed = JSON.parse(init?.body as string);
        lastPutVersion = parsed.version;
        if (putCount === 1) {
          return Promise.resolve({
            ok: false,
            status: 409,
            json: () =>
              Promise.resolve({
                error: { code: "conflict", message: "changed elsewhere" },
                draft: CURRENT_ON_SERVER,
              }),
          });
        }
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              draft: {
                ...CURRENT_ON_SERVER,
                version: CURRENT_ON_SERVER.version + 1,
              },
            }),
        });
      }
      if (url.includes("/api/v1/messages/drafts") && method === "GET") {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              drafts: [FAKE_DRAFT],
              total: 1,
              page: 1,
              per_page: 25,
            }),
        });
      }
      return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
    });

    const user = userEvent.setup();
    renderMessagesPage();
    await waitFor(() => {
      expect(screen.getByText("Hello there")).toBeInTheDocument();
    });

    await user.click(screen.getByText("Drafts"));
    await waitFor(() => {
      expect(screen.getByText("unfinished thought")).toBeInTheDocument();
    });
    await user.click(screen.getByText("unfinished thought"));
    await waitFor(() => {
      expect(screen.getByText("Update Draft")).toBeInTheDocument();
    });

    await user.click(screen.getByText("Update Draft"));
    await waitFor(() => {
      expect(screen.getByText("Load latest version")).toBeInTheDocument();
    });

    await user.click(screen.getByText("Load latest version"));

    await waitFor(() => {
      expect(screen.getByLabelText("Subject")).toHaveValue(
        CURRENT_ON_SERVER.subject,
      );
    });
    expect(screen.queryByText("Load latest version")).not.toBeInTheDocument();

    await user.click(screen.getByText("Update Draft"));
    await waitFor(() => {
      expect(screen.getByText("Draft saved.")).toBeInTheDocument();
    });
    expect(lastPutVersion).toBe(CURRENT_ON_SERVER.version);
  });

  // loadConflictingVersion has a template-specific branch (it skips the
  // receiver/parent reset a draft conflict applies) — worth its own test
  // rather than assuming the draft coverage above implies it works too.
  test("a stale Save as Template is rejected with a 409 and Load latest version resolves it", async () => {
    const CURRENT_TEMPLATE_ON_SERVER = {
      ...FAKE_TEMPLATE,
      subject: "someone else's newer template subject",
      body: "someone else's newer template body",
      version: FAKE_TEMPLATE.version + 1,
    };
    let putCount = 0;
    let lastPutVersion: number | undefined;
    mockFetch.mockImplementation((url: string, init?: RequestInit) => {
      const method = init?.method ?? "GET";
      if (url.includes("/unread-count")) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ unread_count: 0 }),
        });
      }
      if (url.includes("/inbox")) {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              messages: FAKE_INBOX,
              total: 2,
              page: 1,
              per_page: 25,
            }),
        });
      }
      if (/\/api\/v1\/messages\/templates\/5$/.test(url) && method === "GET") {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ template: FAKE_TEMPLATE }),
        });
      }
      if (/\/api\/v1\/messages\/templates\/5$/.test(url) && method === "PUT") {
        putCount += 1;
        const parsed = JSON.parse(init?.body as string);
        lastPutVersion = parsed.version;
        if (putCount === 1) {
          return Promise.resolve({
            ok: false,
            status: 409,
            json: () =>
              Promise.resolve({
                error: { code: "conflict", message: "changed elsewhere" },
                template: CURRENT_TEMPLATE_ON_SERVER,
              }),
          });
        }
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              template: {
                ...CURRENT_TEMPLATE_ON_SERVER,
                version: CURRENT_TEMPLATE_ON_SERVER.version + 1,
              },
            }),
        });
      }
      if (url.includes("/api/v1/messages/templates") && method === "GET") {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              templates: [FAKE_TEMPLATE],
              total: 1,
              page: 1,
              per_page: 25,
            }),
        });
      }
      return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
    });

    const user = userEvent.setup();
    renderMessagesPage();
    await waitFor(() => {
      expect(screen.getByText("Hello there")).toBeInTheDocument();
    });

    await user.click(screen.getByText("Templates"));
    await waitFor(() => {
      expect(screen.getByText("Welcome")).toBeInTheDocument();
    });
    await user.click(screen.getByText("Welcome"));
    await waitFor(() => {
      expect(screen.getByText("Update Template")).toBeInTheDocument();
    });

    await user.click(screen.getByText("Update Template"));
    await waitFor(() => {
      expect(screen.getByText("Load latest version")).toBeInTheDocument();
    });
    // The recipient field must stay untouched by a template conflict — a
    // template never has one to begin with.
    expect(screen.getByLabelText("To")).toHaveValue("");

    await user.click(screen.getByText("Load latest version"));
    await waitFor(() => {
      expect(screen.getByLabelText("Subject")).toHaveValue(
        CURRENT_TEMPLATE_ON_SERVER.subject,
      );
    });
    expect(screen.queryByText("Load latest version")).not.toBeInTheDocument();

    await user.click(screen.getByText("Update Template"));
    await waitFor(() => {
      expect(screen.getByText("Template saved.")).toBeInTheDocument();
    });
    expect(lastPutVersion).toBe(CURRENT_TEMPLATE_ON_SERVER.version);
  });
});
