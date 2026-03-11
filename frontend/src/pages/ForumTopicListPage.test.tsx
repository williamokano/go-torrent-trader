import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, test, expect, vi } from "vitest";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { ForumTopicListPage } from "@/pages/ForumTopicListPage";

vi.mock("@/features/auth/token", () => ({
  getAccessToken: () => "fake-token",
}));

vi.mock("@/config", () => ({
  getConfig: () => ({ API_URL: "http://localhost:8080", SITE_NAME: "Test" }),
}));

const FAKE_RESPONSE = {
  forum: {
    id: 1,
    name: "General Discussion",
    description: "Off-topic chat",
    topic_count: 2,
    post_count: 10,
    min_post_level: 0,
  },
  can_create_topic: true,
  topics: [
    {
      id: 1,
      forum_id: 1,
      user_id: 1,
      username: "alice",
      title: "Pinned Topic",
      pinned: true,
      locked: false,
      post_count: 5,
      view_count: 100,
      last_post_at: "2025-06-01T10:00:00Z",
      last_post_username: "bob",
      created_at: "2025-05-01T10:00:00Z",
    },
    {
      id: 2,
      forum_id: 1,
      user_id: 2,
      username: "bob",
      title: "Regular Topic",
      pinned: false,
      locked: true,
      post_count: 3,
      view_count: 50,
      last_post_at: "2025-06-02T10:00:00Z",
      last_post_username: "alice",
      created_at: "2025-05-02T10:00:00Z",
    },
  ],
  total: 2,
  page: 1,
  per_page: 25,
};

const mockFetch = vi.fn();

afterEach(cleanup);

beforeEach(() => {
  vi.clearAllMocks();
  mockFetch.mockResolvedValue({
    ok: true,
    json: () => Promise.resolve(FAKE_RESPONSE),
  });
  vi.stubGlobal("fetch", mockFetch);
});

function renderPage() {
  return render(
    <MemoryRouter initialEntries={["/forums/1"]}>
      <Routes>
        <Route path="/forums/:id" element={<ForumTopicListPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("ForumTopicListPage", () => {
  test("renders forum name and description", async () => {
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("General Discussion")).toBeInTheDocument();
    });

    expect(screen.getByText("Off-topic chat")).toBeInTheDocument();
  });

  test("renders topic list", async () => {
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Pinned Topic")).toBeInTheDocument();
    });

    expect(screen.getByText("Regular Topic")).toBeInTheDocument();
  });

  test("shows New Topic button", async () => {
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("New Topic")).toBeInTheDocument();
    });
  });

  test("shows empty state", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () =>
        Promise.resolve({
          ...FAKE_RESPONSE,
          topics: [],
          total: 0,
        }),
    });

    renderPage();

    await waitFor(() => {
      expect(
        screen.getByText("No topics yet. Be the first to post!"),
      ).toBeInTheDocument();
    });
  });

  test("hides New Topic button when can_create_topic is false", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () =>
        Promise.resolve({
          ...FAKE_RESPONSE,
          can_create_topic: false,
        }),
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("General Discussion")).toBeInTheDocument();
    });

    expect(screen.queryByText("New Topic")).not.toBeInTheDocument();
  });

  test("shows access denied error", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 403,
    });

    renderPage();

    await waitFor(() => {
      expect(
        screen.getByText(/You do not have access to this forum/),
      ).toBeInTheDocument();
    });
  });
});
