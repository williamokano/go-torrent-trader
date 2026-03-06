import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, test, expect, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { HomePage } from "@/pages/HomePage";

vi.mock("@/features/auth", () => ({
  useAuth: () => ({
    user: null,
    isAuthenticated: false,
    isLoading: false,
    login: vi.fn(),
    logout: vi.fn(),
    register: vi.fn(),
  }),
}));

afterEach(cleanup);

function renderHomePage() {
  return render(
    <MemoryRouter>
      <HomePage />
    </MemoryRouter>,
  );
}

describe("HomePage", () => {
  test("renders welcome message", () => {
    renderHomePage();
    expect(screen.getByText("Welcome to TorrentTrader")).toBeInTheDocument();
  });

  test("renders description for unauthenticated users", () => {
    renderHomePage();
    expect(
      screen.getByText("Your private BitTorrent tracker community."),
    ).toBeInTheDocument();
  });

  test("renders stats section", () => {
    renderHomePage();
    expect(screen.getByLabelText("Site statistics")).toBeInTheDocument();
    expect(screen.getByText("Users")).toBeInTheDocument();
    expect(screen.getByText("Torrents")).toBeInTheDocument();
    expect(screen.getByText("Peers")).toBeInTheDocument();
    expect(screen.getByText("Traffic")).toBeInTheDocument();
  });

  test("renders latest torrents section", () => {
    renderHomePage();
    expect(screen.getByText("Latest Torrents")).toBeInTheDocument();
    expect(screen.getByText("Ubuntu 24.04 LTS Desktop")).toBeInTheDocument();
  });

  test("renders freeleech badges", () => {
    renderHomePage();
    const badges = screen.getAllByText("FREE");
    expect(badges.length).toBeGreaterThan(0);
  });
});
