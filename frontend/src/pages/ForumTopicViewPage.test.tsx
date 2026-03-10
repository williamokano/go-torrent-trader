import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, test, expect, vi } from "vitest";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { ForumTopicViewPage } from "@/pages/ForumTopicViewPage";

vi.mock("@/features/auth/token", () => ({
  getAccessToken: () => "fake-token",
}));
vi.mock("@/config", () => ({
  getConfig: () => ({ API_URL: "http://localhost:8080", SITE_NAME: "Test" }),
}));
const mockUseAuth = vi.fn();
vi.mock("@/features/auth", () => ({ useAuth: () => mockUseAuth() }));
vi.mock("@/components/UsernameDisplay", () => ({
  UsernameDisplay: ({ username }: { userId: number; username: string }) => <span>{username}</span>,
}));
vi.mock("@/components/MarkdownRenderer", () => ({
  MarkdownRenderer: ({ content }: { content: string }) => <div>{content}</div>,
}));
vi.mock("@/components/Pagination", () => ({
  Pagination: () => <div data-testid="pagination" />,
}));

const FAKE_RESPONSE = {
  topic: { id: 1, forum_id: 1, user_id: 1, username: "alice", title: "Test Topic", pinned: false, locked: false, post_count: 2, view_count: 50, forum_name: "General Discussion", created_at: "2025-05-01T10:00:00Z" },
  posts: [
    { id: 1, topic_id: 1, user_id: 1, username: "alice", group_name: "User", body: "First post body", created_at: "2025-05-01T10:00:00Z", user_created_at: "2025-01-01T00:00:00Z", user_post_count: 10 },
    { id: 2, topic_id: 1, user_id: 2, username: "bob", group_name: "User", body: "Reply body", created_at: "2025-05-02T10:00:00Z", user_created_at: "2025-02-01T00:00:00Z", user_post_count: 5 },
  ],
  total: 2, page: 1, per_page: 25,
};
const LOCKED_RESPONSE = { ...FAKE_RESPONSE, topic: { ...FAKE_RESPONSE.topic, locked: true } };
const mockFetch = vi.fn();

afterEach(cleanup);
beforeEach(() => {
  vi.clearAllMocks();
  mockUseAuth.mockReturnValue({ user: { id: 1, username: "testuser", isAdmin: false }, isAuthenticated: true });
  mockFetch.mockResolvedValue({ ok: true, json: () => Promise.resolve(FAKE_RESPONSE) });
  vi.stubGlobal("fetch", mockFetch);
});

function renderPage() {
  return render(
    <MemoryRouter initialEntries={["/forums/topics/1"]}>
      <Routes><Route path="/forums/topics/:id" element={<ForumTopicViewPage />} /></Routes>
    </MemoryRouter>,
  );
}

describe("ForumTopicViewPage", () => {
  test("renders topic title and posts", async () => {
    renderPage();
    await waitFor(() => { expect(screen.getByText("Test Topic")).toBeInTheDocument(); });
    expect(screen.getByText("First post body")).toBeInTheDocument();
    expect(screen.getByText("Reply body")).toBeInTheDocument();
  });

  test("shows reply form when topic is not locked", async () => {
    renderPage();
    await waitFor(() => { expect(screen.getByText("Test Topic")).toBeInTheDocument(); });
    expect(screen.getByText("Post a Reply")).toBeInTheDocument();
  });

  test("hides reply form when topic is locked", async () => {
    mockFetch.mockResolvedValueOnce({ ok: true, json: () => Promise.resolve(LOCKED_RESPONSE) });
    renderPage();
    await waitFor(() => { expect(screen.getByText("Test Topic")).toBeInTheDocument(); });
    expect(screen.getByText("This topic is locked. No new replies can be posted.")).toBeInTheDocument();
    expect(screen.queryByText("Post a Reply")).not.toBeInTheDocument();
  });

  test("hides reply form when user is not logged in", async () => {
    mockUseAuth.mockReturnValue({ user: null, isAuthenticated: false });
    renderPage();
    await waitFor(() => { expect(screen.getByText("Test Topic")).toBeInTheDocument(); });
    expect(screen.queryByText("Post a Reply")).not.toBeInTheDocument();
  });
});
