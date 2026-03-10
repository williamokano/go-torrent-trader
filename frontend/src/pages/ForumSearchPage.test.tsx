import {
  cleanup,
  render,
  screen,
  waitFor,
  fireEvent,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, test, expect, vi } from "vitest";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { ForumSearchPage } from "@/pages/ForumSearchPage";

vi.mock("@/features/auth/token", () => ({
  getAccessToken: () => "fake-token",
}));
vi.mock("@/config", () => ({
  getConfig: () => ({ API_URL: "http://localhost:8080", SITE_NAME: "Test" }),
}));
vi.mock("@/components/UsernameDisplay", () => ({
  UsernameDisplay: ({ username }: { userId: number; username: string }) => (
    <span>{username}</span>
  ),
}));
vi.mock("@/components/Pagination", () => ({
  Pagination: () => <div data-testid="pagination" />,
}));

const FORUMS_RESPONSE = {
  categories: [
    {
      id: 1,
      name: "General",
      forums: [
        { id: 1, name: "Announcements" },
        { id: 2, name: "Help" },
      ],
    },
  ],
};

const SEARCH_RESPONSE = {
  results: [
    {
      post_id: 10,
      body: "This is a test post body with some content",
      topic_id: 5,
      topic_title: "Test Topic Title",
      forum_id: 1,
      forum_name: "Announcements",
      user_id: 1,
      username: "alice",
      created_at: "2025-06-01T10:00:00Z",
    },
    {
      post_id: 11,
      body: "Another result body",
      topic_id: 6,
      topic_title: "Another Topic",
      forum_id: 2,
      forum_name: "Help",
      user_id: 2,
      username: "bob",
      created_at: "2025-06-02T12:00:00Z",
    },
  ],
  total: 2,
  page: 1,
  per_page: 25,
};

const EMPTY_RESPONSE = { results: [], total: 0, page: 1, per_page: 25 };

const mockFetch = vi.fn();

afterEach(cleanup);
beforeEach(() => {
  vi.clearAllMocks();
  vi.useFakeTimers({ shouldAdvanceTime: true });
  mockFetch.mockImplementation((url: string) => {
    if (url.includes("/api/v1/forums/search")) {
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve(SEARCH_RESPONSE),
      });
    }
    if (url.includes("/api/v1/forums")) {
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve(FORUMS_RESPONSE),
      });
    }
    return Promise.resolve({ ok: false });
  });
  vi.stubGlobal("fetch", mockFetch);
});

afterEach(() => {
  vi.useRealTimers();
});

function renderPage(initialEntry = "/forums/search") {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <Routes>
        <Route path="/forums/search" element={<ForumSearchPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("ForumSearchPage", () => {
  test("renders search input and breadcrumb", async () => {
    renderPage();
    expect(screen.getByText("Forum Search")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("Search forums...")).toBeInTheDocument();
    expect(screen.getByText("Forums")).toBeInTheDocument();
  });

  test("shows results when query is provided via URL", async () => {
    renderPage("/forums/search?q=test");
    await waitFor(() => {
      expect(screen.getByText("Test Topic Title")).toBeInTheDocument();
    });
    expect(screen.getByText("Another Topic")).toBeInTheDocument();
    expect(screen.getByText("alice")).toBeInTheDocument();
    expect(screen.getByText("bob")).toBeInTheDocument();
    expect(screen.getByText(/2 results/)).toBeInTheDocument();
  });

  test("shows empty state when no results found", async () => {
    mockFetch.mockImplementation((url: string) => {
      if (url.includes("/api/v1/forums/search")) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve(EMPTY_RESPONSE),
        });
      }
      if (url.includes("/api/v1/forums")) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve(FORUMS_RESPONSE),
        });
      }
      return Promise.resolve({ ok: false });
    });

    renderPage("/forums/search?q=nonexistent");
    await waitFor(() => {
      expect(screen.getByText(/No results found/)).toBeInTheDocument();
    });
  });

  test("shows loading state during search", async () => {
    let resolveSearch: (value: unknown) => void;
    mockFetch.mockImplementation((url: string) => {
      if (url.includes("/api/v1/forums/search")) {
        return new Promise((resolve) => {
          resolveSearch = resolve;
        });
      }
      if (url.includes("/api/v1/forums")) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve(FORUMS_RESPONSE),
        });
      }
      return Promise.resolve({ ok: false });
    });

    renderPage("/forums/search?q=test");
    expect(screen.getByText("Searching...")).toBeInTheDocument();

    resolveSearch!({ ok: true, json: () => Promise.resolve(SEARCH_RESPONSE) });
    await waitFor(() => {
      expect(screen.queryByText("Searching...")).not.toBeInTheDocument();
    });
  });

  test("submits search on form submit", async () => {
    renderPage();
    const input = screen.getByPlaceholderText("Search forums...");
    fireEvent.change(input, { target: { value: "hello" } });
    fireEvent.submit(input.closest("form")!);

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining("q=hello"),
        expect.any(Object),
      );
    });
  });

  test("renders forum filter dropdown", async () => {
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Announcements")).toBeInTheDocument();
    });
    expect(screen.getByText("Help")).toBeInTheDocument();
    expect(screen.getByText("All Forums")).toBeInTheDocument();
  });
});
