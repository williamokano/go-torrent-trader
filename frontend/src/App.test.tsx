import { render, screen } from "@testing-library/react";
import { test } from "vitest";
import { ThemeProvider } from "@/themes";
import App from "@/App";

test("renders welcome message", () => {
  render(
    <ThemeProvider>
      <App />
    </ThemeProvider>,
  );
  screen.getByText("Welcome to TorrentTrader 3.0");
});
