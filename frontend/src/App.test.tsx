import { render, screen } from "@testing-library/react";
import { test } from "vitest";
import App from "./App";

test("renders welcome message", () => {
  render(<App />);
  screen.getByText("Welcome to TorrentTrader 3.0");
});
