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
