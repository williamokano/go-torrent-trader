import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { NewsDetailPage } from "./NewsDetailPage";

vi.mock("@/config", () => ({
  getConfig: () => ({ API_URL: "http://localhost:8080" }),
}));

const mockArticle = {
  id: 1,
  title: "Welcome to TorrentTrader",
  body: "This is the full content of the news article.",
  author_name: "admin",
  created_at: "2026-03-01T00:00:00Z",
};

describe("NewsDetailPage", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("renders a news article", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue({
      ok: true,
      json: async () => ({ article: mockArticle }),
    } as Response);

    render(
      <MemoryRouter initialEntries={["/news/1"]}>
        <Routes>
          <Route path="/news/:id" element={<NewsDetailPage />} />
        </Routes>
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByText("Welcome to TorrentTrader")).toBeInTheDocument();
    });

    expect(
      screen.getByText("This is the full content of the news article."),
    ).toBeInTheDocument();
    expect(screen.getByText(/admin/)).toBeInTheDocument();
    expect(screen.getByText("Back to News")).toBeInTheDocument();
  });

  it("shows error when article not found", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue({
      ok: false,
      status: 404,
      json: async () => ({ error: { message: "not found" } }),
    } as Response);

    render(
      <MemoryRouter initialEntries={["/news/999"]}>
        <Routes>
          <Route path="/news/:id" element={<NewsDetailPage />} />
        </Routes>
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByText("Article not found")).toBeInTheDocument();
    });
  });
});
