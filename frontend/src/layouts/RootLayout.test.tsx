import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, test, expect } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { ThemeProvider } from "@/themes";
import { RootLayout } from "@/layouts/RootLayout";

afterEach(cleanup);

function renderLayout() {
  return render(
    <ThemeProvider>
      <MemoryRouter>
        <RootLayout />
      </MemoryRouter>
    </ThemeProvider>,
  );
}

describe("RootLayout", () => {
  test("renders header with site name", () => {
    renderLayout();
    expect(screen.getByText("TorrentTrader")).toBeInTheDocument();
  });

  test("renders navigation links", () => {
    renderLayout();
    expect(screen.getByText("Home")).toBeInTheDocument();
    expect(screen.getByText("Browse")).toBeInTheDocument();
    expect(screen.getByText("Forums")).toBeInTheDocument();
    expect(screen.getByText("Upload")).toBeInTheDocument();
  });

  test("renders theme toggle button", () => {
    renderLayout();
    const button = document.querySelector(".header__theme-btn");
    expect(button).toBeInTheDocument();
  });

  test("renders footer with stats placeholder", () => {
    renderLayout();
    expect(screen.getByText(/Torrents:/)).toBeInTheDocument();
  });

  test("renders footer links", () => {
    renderLayout();
    expect(screen.getByText("About")).toBeInTheDocument();
    expect(screen.getByText("FAQ")).toBeInTheDocument();
  });
});
