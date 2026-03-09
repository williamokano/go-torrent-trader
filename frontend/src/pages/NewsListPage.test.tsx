import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { NewsListPage } from "./NewsListPage";

vi.mock("@/config", () => ({
  getConfig: () => ({ API_URL: "http://localhost:8080" }),
}));

const mockArticles = [
  {
    id: 1,
    title: "Welcome to TorrentTrader",
    body: "This is the first news article with some content that should be shown as a preview.",
    author_name: "admin",
    created_at: "2026-03-01T00:00:00Z",
  },
  {
    id: 2,
    title: "Site Update",
    body: "We have updated the site with new features.",
    author_name: "admin",
    created_at: "2026-03-05T00:00:00Z",
  },
];

describe("NewsListPage", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("renders published news articles", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue({
      ok: true,
      json: async () => ({
        articles: mockArticles,
        total: 2,
        page: 1,
        per_page: 10,
      }),
    } as Response);

    render(
      <MemoryRouter>
        <NewsListPage />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByText("Welcome to TorrentTrader")).toBeInTheDocument();
    });

    expect(screen.getByText("Site Update")).toBeInTheDocument();
    const readMoreLinks = screen.getAllByText("Read more");
    expect(readMoreLinks.length).toBe(2);
  });

  it("shows empty state when no articles", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue({
      ok: true,
      json: async () => ({
        articles: [],
        total: 0,
        page: 1,
        per_page: 10,
      }),
    } as Response);

    render(
      <MemoryRouter>
        <NewsListPage />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByText("No news articles yet.")).toBeInTheDocument();
    });
  });
});
