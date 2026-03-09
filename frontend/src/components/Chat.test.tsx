import { cleanup, render, screen, fireEvent } from "@testing-library/react";
import { afterEach, describe, test, expect, vi, beforeEach } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { ToastProvider } from "@/components/toast";
import { ChatProvider } from "@/lib/ChatProvider";
import { Chat } from "./Chat";

// jsdom doesn't implement scrollIntoView
Element.prototype.scrollIntoView = vi.fn();

// Mock auth
const mockAuth = {
  user: null as {
    id: number;
    username: string;
    isStaff: boolean;
    isAdmin: boolean;
  } | null,
  isAuthenticated: false,
};

vi.mock("@/features/auth", () => ({
  useAuth: () => mockAuth,
}));

vi.mock("@/features/auth/token", () => ({
  getAccessToken: vi.fn(() => "test-token"),
}));

vi.mock("@/config", () => ({
  getConfig: () => ({ API_URL: "http://localhost:8080" }),
}));

// Mock WebSocket
class MockWebSocket {
  static instances: MockWebSocket[] = [];
  url: string;
  readyState = 1; // OPEN
  onopen: (() => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  sentMessages: string[] = [];

  constructor(url: string) {
    this.url = url;
    MockWebSocket.instances.push(this);
    // Simulate connection after a tick
    setTimeout(() => this.onopen?.(), 0);
  }

  send(data: string) {
    this.sentMessages.push(data);
  }

  close() {
    this.readyState = 3;
  }
}

beforeEach(() => {
  MockWebSocket.instances = [];
  vi.stubGlobal("WebSocket", MockWebSocket);
  // Mock fetch for mute-status endpoint (default: not muted)
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string) => {
      if (typeof url === "string" && url.includes("/chat/mute-status")) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ muted: false }),
        });
      }
      return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
    }),
  );
  // Reset auth state
  mockAuth.user = null;
  mockAuth.isAuthenticated = false;
});

afterEach(cleanup);

function renderChat() {
  return render(
    <MemoryRouter>
      <ToastProvider>
        <ChatProvider>
          <Chat />
        </ChatProvider>
      </ToastProvider>
    </MemoryRouter>,
  );
}

describe("Chat", () => {
  test("renders nothing when not authenticated", () => {
    renderChat();
    expect(screen.queryByText("Shoutbox")).toBeNull();
  });

  test("renders collapsed chat when authenticated", () => {
    mockAuth.user = {
      id: 1,
      username: "alice",
      isStaff: false,
      isAdmin: false,
    };
    mockAuth.isAuthenticated = true;

    renderChat();
    expect(screen.getByText("Shoutbox")).toBeInTheDocument();
    // Should not show input area when collapsed
    expect(screen.queryByPlaceholderText("Type a message...")).toBeNull();
  });

  test("expands when header is clicked", () => {
    mockAuth.user = {
      id: 1,
      username: "alice",
      isStaff: false,
      isAdmin: false,
    };
    mockAuth.isAuthenticated = true;

    renderChat();
    fireEvent.click(screen.getByText("Shoutbox"));
    expect(
      screen.getByPlaceholderText("Type a message..."),
    ).toBeInTheDocument();
  });

  test("shows empty state when expanded with no messages", () => {
    mockAuth.user = {
      id: 1,
      username: "alice",
      isStaff: false,
      isAdmin: false,
    };
    mockAuth.isAuthenticated = true;

    renderChat();
    fireEvent.click(screen.getByText("Shoutbox"));
    expect(
      screen.getByText("No messages yet. Be the first to say something!"),
    ).toBeInTheDocument();
  });

  test("connects to WebSocket with token on mount", async () => {
    mockAuth.user = {
      id: 1,
      username: "alice",
      isStaff: false,
      isAdmin: false,
    };
    mockAuth.isAuthenticated = true;

    renderChat();

    // Wait for effect to fire
    await vi.waitFor(() => {
      expect(MockWebSocket.instances.length).toBe(1);
    });

    const ws = MockWebSocket.instances[0];
    expect(ws.url).toBe("ws://localhost:8080/ws/chat?token=test-token");
  });

  test("does not show delete button for non-staff users", async () => {
    mockAuth.user = {
      id: 1,
      username: "alice",
      isStaff: false,
      isAdmin: false,
    };
    mockAuth.isAuthenticated = true;

    renderChat();
    fireEvent.click(screen.getByText("Shoutbox"));

    await vi.waitFor(() => {
      expect(MockWebSocket.instances.length).toBe(1);
    });

    // Simulate backfill
    const ws = MockWebSocket.instances[0];
    ws.onmessage?.({
      data: JSON.stringify({
        type: "backfill",
        messages: [
          {
            id: 1,
            user_id: 2,
            username: "bob",
            message: "hello",
            created_at: new Date().toISOString(),
          },
        ],
      }),
    });

    await vi.waitFor(() => {
      expect(screen.getByText("hello")).toBeInTheDocument();
    });

    // No delete button for non-staff
    expect(screen.queryByTitle("Delete message")).toBeNull();
  });

  test("shows delete button for staff users", async () => {
    mockAuth.user = {
      id: 1,
      username: "admin",
      isStaff: true,
      isAdmin: true,
    };
    mockAuth.isAuthenticated = true;

    renderChat();
    fireEvent.click(screen.getByText("Shoutbox"));

    await vi.waitFor(() => {
      expect(MockWebSocket.instances.length).toBe(1);
    });

    const ws = MockWebSocket.instances[0];
    ws.onmessage?.({
      data: JSON.stringify({
        type: "backfill",
        messages: [
          {
            id: 1,
            user_id: 2,
            username: "bob",
            message: "hello",
            created_at: new Date().toISOString(),
          },
        ],
      }),
    });

    await vi.waitFor(() => {
      expect(screen.getByText("hello")).toBeInTheDocument();
    });

    expect(screen.getByTitle("Delete message")).toBeInTheDocument();
  });

  test("staff sees moderation menu on username", async () => {
    mockAuth.user = {
      id: 1,
      username: "admin",
      isStaff: true,
      isAdmin: true,
    };
    mockAuth.isAuthenticated = true;

    renderChat();
    fireEvent.click(screen.getByText("Shoutbox"));

    await vi.waitFor(() => {
      expect(MockWebSocket.instances.length).toBe(1);
    });

    const ws = MockWebSocket.instances[0];
    ws.onmessage?.({
      data: JSON.stringify({
        type: "backfill",
        messages: [
          {
            id: 1,
            user_id: 2,
            username: "bob",
            message: "hello",
            created_at: new Date().toISOString(),
          },
        ],
      }),
    });

    await vi.waitFor(() => {
      expect(screen.getByText("hello")).toBeInTheDocument();
    });

    // Username is a moderation trigger button for staff
    const userButton = screen.getByTitle("Moderation actions");
    expect(userButton).toBeInTheDocument();
    expect(userButton.textContent).toBe("bob");

    // Click opens dropdown with moderation actions
    fireEvent.click(userButton);
    expect(screen.getByText("Delete all messages")).toBeInTheDocument();
    expect(screen.getByText("Mute user")).toBeInTheDocument();
    expect(screen.getByText("Unmute user")).toBeInTheDocument();
  });

  test("non-staff sees regular link for username", async () => {
    mockAuth.user = {
      id: 1,
      username: "alice",
      isStaff: false,
      isAdmin: false,
    };
    mockAuth.isAuthenticated = true;

    renderChat();
    fireEvent.click(screen.getByText("Shoutbox"));

    await vi.waitFor(() => {
      expect(MockWebSocket.instances.length).toBe(1);
    });

    const ws = MockWebSocket.instances[0];
    ws.onmessage?.({
      data: JSON.stringify({
        type: "backfill",
        messages: [
          {
            id: 1,
            user_id: 2,
            username: "bob",
            message: "hello",
            created_at: new Date().toISOString(),
          },
        ],
      }),
    });

    await vi.waitFor(() => {
      expect(screen.getByText("hello")).toBeInTheDocument();
    });

    // Username is a regular link, not a moderation menu
    expect(screen.queryByTitle("Moderation actions")).toBeNull();
    const userLink = screen.getByText("bob");
    expect(userLink.closest("a")).toBeTruthy();
  });

  test("removes message on delete broadcast", async () => {
    mockAuth.user = {
      id: 1,
      username: "alice",
      isStaff: false,
      isAdmin: false,
    };
    mockAuth.isAuthenticated = true;

    renderChat();
    fireEvent.click(screen.getByText("Shoutbox"));

    await vi.waitFor(() => {
      expect(MockWebSocket.instances.length).toBe(1);
    });

    const ws = MockWebSocket.instances[0];

    // Send backfill with a message
    ws.onmessage?.({
      data: JSON.stringify({
        type: "backfill",
        messages: [
          {
            id: 42,
            user_id: 2,
            username: "bob",
            message: "to be deleted",
            created_at: new Date().toISOString(),
          },
        ],
      }),
    });

    await vi.waitFor(() => {
      expect(screen.getByText("to be deleted")).toBeInTheDocument();
    });

    // Send delete event
    ws.onmessage?.({
      data: JSON.stringify({ type: "delete", id: 42 }),
    });

    await vi.waitFor(() => {
      expect(screen.queryByText("to be deleted")).toBeNull();
    });
  });

  test("disables input and shows muted notice on mute event", async () => {
    mockAuth.user = {
      id: 1,
      username: "alice",
      isStaff: false,
      isAdmin: false,
    };
    mockAuth.isAuthenticated = true;

    renderChat();
    fireEvent.click(screen.getByText("Shoutbox"));

    await vi.waitFor(() => {
      expect(MockWebSocket.instances.length).toBe(1);
    });

    const ws = MockWebSocket.instances[0];

    // Send mute event
    const expiresAt = new Date(Date.now() + 10 * 60 * 1000).toISOString();
    ws.onmessage?.({
      data: JSON.stringify({
        type: "mute",
        expires_at: expiresAt,
        reason: "spam",
      }),
    });

    await vi.waitFor(() => {
      const input = screen.getByPlaceholderText("You are muted");
      expect(input).toBeDisabled();
    });

    expect(screen.getByText(/You are muted/)).toBeInTheDocument();
  });

  test("re-enables input on unmute event", async () => {
    mockAuth.user = {
      id: 1,
      username: "alice",
      isStaff: false,
      isAdmin: false,
    };
    mockAuth.isAuthenticated = true;

    renderChat();
    fireEvent.click(screen.getByText("Shoutbox"));

    await vi.waitFor(() => {
      expect(MockWebSocket.instances.length).toBe(1);
    });

    const ws = MockWebSocket.instances[0];

    // First mute the user
    const expiresAt = new Date(Date.now() + 10 * 60 * 1000).toISOString();
    ws.onmessage?.({
      data: JSON.stringify({
        type: "mute",
        expires_at: expiresAt,
        reason: "spam",
      }),
    });

    await vi.waitFor(() => {
      expect(screen.getByPlaceholderText("You are muted")).toBeDisabled();
    });

    // Now unmute
    ws.onmessage?.({
      data: JSON.stringify({ type: "unmute" }),
    });

    await vi.waitFor(() => {
      const input = screen.getByPlaceholderText("Type a message...");
      expect(input).not.toBeDisabled();
    });
  });

  test("removes all user messages on delete_user broadcast", async () => {
    mockAuth.user = {
      id: 1,
      username: "alice",
      isStaff: false,
      isAdmin: false,
    };
    mockAuth.isAuthenticated = true;

    renderChat();
    fireEvent.click(screen.getByText("Shoutbox"));

    await vi.waitFor(() => {
      expect(MockWebSocket.instances.length).toBe(1);
    });

    const ws = MockWebSocket.instances[0];

    ws.onmessage?.({
      data: JSON.stringify({
        type: "backfill",
        messages: [
          {
            id: 1,
            user_id: 2,
            username: "bob",
            message: "bob msg 1",
            created_at: new Date().toISOString(),
          },
          {
            id: 2,
            user_id: 2,
            username: "bob",
            message: "bob msg 2",
            created_at: new Date().toISOString(),
          },
          {
            id: 3,
            user_id: 3,
            username: "carol",
            message: "carol msg",
            created_at: new Date().toISOString(),
          },
        ],
      }),
    });

    await vi.waitFor(() => {
      expect(screen.getByText("bob msg 1")).toBeInTheDocument();
      expect(screen.getByText("bob msg 2")).toBeInTheDocument();
      expect(screen.getByText("carol msg")).toBeInTheDocument();
    });

    // Send delete_user event for bob
    ws.onmessage?.({
      data: JSON.stringify({ type: "delete_user", user_id: 2 }),
    });

    await vi.waitFor(() => {
      expect(screen.queryByText("bob msg 1")).toBeNull();
      expect(screen.queryByText("bob msg 2")).toBeNull();
      // Carol's message should remain
      expect(screen.getByText("carol msg")).toBeInTheDocument();
    });
  });
});
