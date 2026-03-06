import { cleanup, render, screen } from "@testing-library/react";
import { userEvent } from "@testing-library/user-event";
import { afterEach, describe, test, expect } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { BrowsePage } from "@/pages/BrowsePage";

afterEach(cleanup);

function renderBrowsePage(initialEntries = ["/browse"]) {
  return render(
    <MemoryRouter initialEntries={initialEntries}>
      <BrowsePage />
    </MemoryRouter>,
  );
}

describe("BrowsePage", () => {
  test("renders page title", () => {
    renderBrowsePage();
    expect(screen.getByText("Browse Torrents")).toBeInTheDocument();
  });

  test("renders torrent table with data", () => {
    renderBrowsePage();
    expect(screen.getByText("Ubuntu 24.04 LTS Desktop")).toBeInTheDocument();
    expect(screen.getByText("Arch Linux 2026.03.01")).toBeInTheDocument();
  });

  test("renders search input", () => {
    renderBrowsePage();
    expect(
      screen.getByPlaceholderText("Search torrents..."),
    ).toBeInTheDocument();
  });

  test("renders category filter", () => {
    renderBrowsePage();
    expect(screen.getByLabelText("Category")).toBeInTheDocument();
  });

  test("renders sort select", () => {
    renderBrowsePage();
    expect(screen.getByLabelText("Sort by")).toBeInTheDocument();
  });

  test("renders pagination", () => {
    renderBrowsePage();
    expect(screen.getByLabelText("Pagination")).toBeInTheDocument();
  });

  test("renders freeleech badges", () => {
    renderBrowsePage();
    const badges = screen.getAllByText("FREE");
    expect(badges.length).toBeGreaterThan(0);
  });

  test("filters by search query", async () => {
    renderBrowsePage();
    const user = userEvent.setup();
    const searchInput = screen.getByPlaceholderText("Search torrents...");
    await user.type(searchInput, "Ubuntu");
    expect(screen.getByText("Ubuntu 24.04 LTS Desktop")).toBeInTheDocument();
    expect(screen.queryByText("Arch Linux 2026.03.01")).not.toBeInTheDocument();
  });

  test("shows empty state when no results", async () => {
    renderBrowsePage();
    const user = userEvent.setup();
    const searchInput = screen.getByPlaceholderText("Search torrents...");
    await user.type(searchInput, "nonexistent torrent xyz");
    expect(screen.getByText("No torrents found.")).toBeInTheDocument();
  });

  test("renders health indicators", () => {
    renderBrowsePage();
    const healthDots = document.querySelectorAll(".browse__health");
    expect(healthDots.length).toBeGreaterThan(0);
  });
});
