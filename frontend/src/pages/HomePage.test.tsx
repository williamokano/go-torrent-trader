import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, test, expect } from "vitest";
import { HomePage } from "@/pages/HomePage";

afterEach(cleanup);

describe("HomePage", () => {
  test("renders welcome message", () => {
    render(<HomePage />);
    expect(screen.getByText("Welcome to TorrentTrader")).toBeInTheDocument();
  });

  test("renders description", () => {
    render(<HomePage />);
    expect(
      screen.getByText("Your private BitTorrent tracker community."),
    ).toBeInTheDocument();
  });
});
