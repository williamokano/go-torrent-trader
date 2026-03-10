import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, test, expect, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { ForumIndexPage } from "@/pages/ForumIndexPage";

vi.mock("@/features/auth/token", () => ({
  getAccessToken: () => "fake-token",
}));

vi.mock("@/config", () => ({
  getConfig: () => ({ API_URL: "http://localhost:8080", SITE_NAME: "Test" }),
}));

const FAKE_CATEGORIES = [
  {
    id: 1,
    name: "General",
    forums: [
      {
        id: 1,
        name: "Announcements",
        description: "Site news and announcements",
        topic_count: 5,
        post_count: 20,
        last_post_at: "2025-06-01T10:00:00Z",
        last_post_username: "admin",
        last_post_topic_id: 1,
        last_post_topic_title: "Welcome to the forums",
      },
      {
        id: 2,
        name: "General Discussion",
        description: "Off-topic chat",
        topic_count: 0,
        post_count: 0,
      },
    ],
  },
];

const mockFetch = vi.fn();

afterEach(cleanup);

beforeEach(() => {
  vi.clearAllMocks();
  mockFetch.mockResolvedValue({
    ok: true,
    json: () => Promise.resolve({ categories: FAKE_CATEGORIES }),
  });
  vi.stubGlobal("fetch", mockFetch);
});

function renderPage() {
  return render(
    <MemoryRouter initialEntries={["/forums"]}>
      <ForumIndexPage />
    </MemoryRouter>,
  );
}

describe("ForumIndexPage", () => {
  test("renders forum categories and forums", async () => {
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("General")).toBeInTheDocument();
    });

    expect(screen.getByText("Announcements")).toBeInTheDocument();
    expect(screen.getByText("General Discussion")).toBeInTheDocument();
    expect(screen.getByText("Site news and announcements")).toBeInTheDocument();
  });

  test("displays topic and post counts", async () => {
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Announcements")).toBeInTheDocument();
    });

    expect(screen.getByText("5")).toBeInTheDocument();
    expect(screen.getByText("20")).toBeInTheDocument();
  });

  test("shows last post info", async () => {
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Welcome to the forums")).toBeInTheDocument();
    });
  });

  test("shows no forums message when empty", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve({ categories: [] }),
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("No forums available.")).toBeInTheDocument();
    });
  });

  test("shows error on fetch failure", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 500,
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText(/Error:/)).toBeInTheDocument();
    });
  });

  test("calls API with auth header", async () => {
    renderPage();

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "http://localhost:8080/api/v1/forums",
        expect.objectContaining({
          headers: { Authorization: "Bearer fake-token" },
        }),
      );
    });
  });
});
