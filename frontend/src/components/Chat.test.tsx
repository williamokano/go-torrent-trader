import { cleanup, render, screen, fireEvent } from "@testing-library/react";
import { afterEach, describe, test, expect, vi, beforeEach } from "vitest";
import { MemoryRouter } from "react-router-dom";
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
  // Reset auth state
  mockAuth.user = null;
  mockAuth.isAuthenticated = false;
});

afterEach(cleanup);

function renderChat() {
  return render(
    <MemoryRouter>
      <Chat />
    </MemoryRouter>,
  );
}

describe("Chat", () => {
  test("renders nothing when not authenticated", () => {
    const { container } = renderChat();
    expect(container.innerHTML).toBe("");
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
});
