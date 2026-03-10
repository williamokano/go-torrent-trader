import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { AdminNewsPage } from "./AdminNewsPage";

vi.mock("@/features/auth/token", () => ({
  getAccessToken: () => "test-token",
}));

vi.mock("@/config", () => ({
  getConfig: () => ({ API_URL: "http://localhost:8080" }),
}));

vi.mock("@/components/toast", () => ({
  useToast: () => ({
    success: vi.fn(),
    error: vi.fn(),
  }),
}));

const mockArticles = [
  {
    id: 1,
    title: "Welcome to TorrentTrader",
    body: "This is the first news article.",
    author_id: 1,
    published: true,
    created_at: "2026-03-01T00:00:00Z",
    updated_at: "2026-03-01T00:00:00Z",
    author_name: "admin",
  },
  {
    id: 2,
    title: "Draft Article",
    body: "This is a draft.",
    author_id: 1,
    published: false,
    created_at: "2026-03-05T00:00:00Z",
    updated_at: "2026-03-05T00:00:00Z",
    author_name: "admin",
  },
];

describe("AdminNewsPage", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it("renders the page with articles table", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue({
      ok: true,
      json: async () => ({
        articles: mockArticles,
        total: 2,
        page: 1,
        per_page: 25,
      }),
    } as Response);

    render(
      <MemoryRouter>
        <AdminNewsPage />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByText("Welcome to TorrentTrader")).toBeInTheDocument();
    });

    expect(screen.getByText("Draft Article")).toBeInTheDocument();
    expect(screen.getByText("Published")).toBeInTheDocument();
    expect(screen.getByText("Draft")).toBeInTheDocument();
  });

  it("shows empty state when no articles", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue({
      ok: true,
      json: async () => ({
        articles: [],
        total: 0,
        page: 1,
        per_page: 25,
      }),
    } as Response);

    render(
      <MemoryRouter>
        <AdminNewsPage />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByText("No news articles found.")).toBeInTheDocument();
    });
  });

  it("renders the Create Article button", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue({
      ok: true,
      json: async () => ({
        articles: [],
        total: 0,
        page: 1,
        per_page: 25,
      }),
    } as Response);

    render(
      <MemoryRouter>
        <AdminNewsPage />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByText("Create Article")).toBeInTheDocument();
    });
  });

  it("shows edit and delete buttons for each article", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue({
      ok: true,
      json: async () => ({
        articles: mockArticles,
        total: 2,
        page: 1,
        per_page: 25,
      }),
    } as Response);

    render(
      <MemoryRouter>
        <AdminNewsPage />
      </MemoryRouter>,
    );

    await waitFor(() => {
      const editButtons = screen.getAllByText("Edit");
      expect(editButtons.length).toBe(2);
      const deleteButtons = screen.getAllByText("Delete");
      expect(deleteButtons.length).toBe(2);
    });
  });
});
