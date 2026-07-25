import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { ToastProvider } from "@/components/toast";
import { ChatProvider } from "@/lib/ChatProvider";
import { Shoutbox } from "./Shoutbox";

// The Shoutbox is what the home page actually mounts — the floating Chat widget
// returns null while it is visible — so the message row needs its own coverage
// rather than relying on Chat.test.tsx exercising the same JSX.

// jsdom doesn't implement scrollIntoView.
Element.prototype.scrollIntoView = vi.fn();

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

class MockWebSocket {
  static instances: MockWebSocket[] = [];
  url: string;
  readyState = 1;
  onopen: (() => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;

  constructor(url: string) {
    this.url = url;
    MockWebSocket.instances.push(this);
    setTimeout(() => this.onopen?.(), 0);
  }

  send() {}
  close() {
    this.readyState = 3;
  }
}

beforeEach(() => {
  MockWebSocket.instances = [];
  vi.stubGlobal("WebSocket", MockWebSocket);
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
  mockAuth.user = { id: 1, username: "admin", isStaff: true, isAdmin: true };
  mockAuth.isAuthenticated = true;
});

afterEach(cleanup);

function renderShoutbox() {
  return render(
    <MemoryRouter>
      <ToastProvider>
        <ChatProvider>
          <Shoutbox />
        </ChatProvider>
      </ToastProvider>
    </MemoryRouter>,
  );
}

async function deliver(message: Record<string, unknown>) {
  await vi.waitFor(() => {
    expect(MockWebSocket.instances.length).toBe(1);
  });
  MockWebSocket.instances[0].onmessage?.({
    data: JSON.stringify({ type: "message", ...message }),
  });
}

describe("Shoutbox", () => {
  test("renders a normal message with a profile link and staff actions", async () => {
    renderShoutbox();

    await deliver({
      id: 1,
      user_id: 2,
      username: "bob",
      message: "hello",
      created_at: new Date().toISOString(),
    });

    await vi.waitFor(() => {
      expect(screen.getByText("hello")).toBeInTheDocument();
    });
    expect(screen.getByTitle("Delete message")).toBeInTheDocument();
  });

  // A system announcement has no author, so it must not offer a profile link or
  // moderation actions that would target a nonexistent user.
  test("renders a system message without user actions", async () => {
    renderShoutbox();

    await deliver({
      id: 7,
      user_id: 0,
      username: "System",
      message: "New torrent: Some.Release-GROUP",
      system: true,
      created_at: new Date().toISOString(),
    });

    await vi.waitFor(() => {
      expect(
        screen.getByText("New torrent: Some.Release-GROUP"),
      ).toBeInTheDocument();
    });

    const label = screen.getByText("System");
    expect(label.tagName).not.toBe("A");
    expect(label).toHaveClass("shoutbox__message-system-label");
    expect(screen.queryByTitle("Delete message")).toBeNull();
  });
});
