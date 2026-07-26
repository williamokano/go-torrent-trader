import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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
  // moderation actions that would target a nonexistent user. The label is
  // whatever the API sent — it is an operator setting, so a hardcoded "System"
  // here would pass while the real page showed the wrong name.
  test("renders a system message without user actions", async () => {
    renderShoutbox();

    await deliver({
      id: 7,
      user_id: 0,
      username: "Tracker Bot",
      message: "New torrent: Some.Release-GROUP",
      system: true,
      created_at: new Date().toISOString(),
    });

    await vi.waitFor(() => {
      expect(
        screen.getByText("New torrent: Some.Release-GROUP"),
      ).toBeInTheDocument();
    });

    const label = screen.getByText("Tracker Bot");
    expect(label.tagName).not.toBe("A");
    expect(label).toHaveClass("shoutbox__message-system-label");
  });

  // Staff must be able to remove an announcement: a mis-worded or duplicated
  // connector line would otherwise sit in the shoutbox forever, and the bulk
  // "delete this user's messages" path cannot reach it either (system rows have
  // no user_id).
  test("lets staff delete a system message", async () => {
    renderShoutbox();

    await deliver({
      id: 9,
      user_id: 0,
      username: "Tracker Bot",
      message: "New torrent: Oops.Wrong.One",
      system: true,
      created_at: new Date().toISOString(),
    });

    await vi.waitFor(() => {
      expect(screen.getByTitle("Delete message")).toBeInTheDocument();
    });

    await userEvent.click(screen.getByTitle("Delete message"));
    await userEvent.click(screen.getByRole("button", { name: "Delete" }));

    await vi.waitFor(() => {
      expect(fetch).toHaveBeenCalledWith(
        "http://localhost:8080/api/v1/admin/chat/messages/9",
        expect.objectContaining({ method: "DELETE" }),
      );
    });
  });

  // The point of the {{.Link}} template field: the connector sends Markdown and
  // the shoutbox has to turn it into a real link, otherwise the announcement
  // reads as literal brackets and parentheses.
  test("renders the announcement's Markdown link as a link", async () => {
    renderShoutbox();

    await deliver({
      id: 13,
      user_id: 0,
      username: "Tracker Bot",
      message:
        "New torrent: [\\[SubsPlease\\] Show - 01](https://tracker.test/torrent/7) — Anime, 1.20 GiB",
      system: true,
      created_at: new Date().toISOString(),
    });

    const link = await screen.findByRole("link", {
      name: "[SubsPlease] Show - 01",
    });
    expect(link).toHaveAttribute("href", "https://tracker.test/torrent/7");
    // The escaped brackets are part of the label, not leftover Markdown.
    expect(screen.queryByText(/\\\[SubsPlease/)).toBeNull();
  });

  test("hides the delete action from non-staff", async () => {
    mockAuth.user = {
      id: 5,
      username: "member",
      isStaff: false,
      isAdmin: false,
    };
    renderShoutbox();

    await deliver({
      id: 11,
      user_id: 0,
      username: "Tracker Bot",
      message: "New torrent: Members.Cannot.Delete",
      system: true,
      created_at: new Date().toISOString(),
    });

    await vi.waitFor(() => {
      expect(
        screen.getByText("New torrent: Members.Cannot.Delete"),
      ).toBeInTheDocument();
    });
    expect(screen.queryByTitle("Delete message")).toBeNull();
  });
});
