import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, test } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { ThemeProvider } from "@/themes";
import { RootLayout } from "@/layouts/RootLayout";
import { HomePage } from "@/pages/HomePage";

afterEach(cleanup);

test("renders app with routing", () => {
  render(
    <ThemeProvider>
      <MemoryRouter initialEntries={["/"]}>
        <RootLayout />
      </MemoryRouter>
    </ThemeProvider>,
  );
  screen.getByText("TorrentTrader");
});

test("renders home page content at root route", () => {
  render(
    <ThemeProvider>
      <MemoryRouter initialEntries={["/"]}>
        <HomePage />
      </MemoryRouter>
    </ThemeProvider>,
  );
  screen.getByText("Welcome to TorrentTrader");
});
