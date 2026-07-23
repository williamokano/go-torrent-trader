import {
  cleanup,
  render,
  screen,
  fireEvent,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { MetadataIssuesPage } from "@/pages/MetadataIssuesPage";

const mockUseAuth = vi.fn();
vi.mock("@/features/auth/useAuth", () => ({
  useAuth: () => mockUseAuth(),
}));
vi.mock("@/config", () => ({
  getConfig: () => ({ API_URL: "http://localhost:8080", SITE_NAME: "Test" }),
}));
vi.mock("@/features/auth/token", () => ({
  getAccessToken: () => "fake-token",
}));

const ISSUES = [
  {
    torrent_id: 5,
    torrent_name: "Old Movie",
    category_id: 1,
    category_name: "Movies",
    uploader_id: 7,
    uploader_name: "bob",
    anonymous: false,
    missing_fields: [{ key: "year", label: "Year" }],
  },
];

function mockFetchIssues() {
  return vi.spyOn(globalThis, "fetch").mockImplementation((url) =>
    Promise.resolve({
      ok: true,
      json: async () => ({
        issues: ISSUES,
        scope: String(url).includes("scope=all") ? "all" : "mine",
      }),
    } as Response),
  );
}

function renderPage() {
  return render(
    <MemoryRouter>
      <MetadataIssuesPage />
    </MemoryRouter>,
  );
}

afterEach(cleanup);
beforeEach(() => {
  vi.clearAllMocks();
  vi.restoreAllMocks();
});

describe("MetadataIssuesPage", () => {
  test("a regular user sees their own issues, scoped to mine, no toggle", async () => {
    mockUseAuth.mockReturnValue({ user: { isAdmin: false } });
    const fetchSpy = mockFetchIssues();

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Old Movie")).toBeInTheDocument();
    });
    // Missing field label + fix link.
    expect(screen.getByText("Year")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Fix" })).toHaveAttribute(
      "href",
      "/torrent/5/edit",
    );
    // No admin scope toggle, no uploader column.
    expect(
      screen.queryByRole("button", { name: "All uploaders" }),
    ).not.toBeInTheDocument();
    expect(screen.queryByText("Uploader")).not.toBeInTheDocument();
    // Fetched the caller's own report.
    expect(String(fetchSpy.mock.calls[0][0])).toContain("scope=mine");
  });

  test("an admin defaults to all uploaders and can switch to mine", async () => {
    mockUseAuth.mockReturnValue({ user: { isAdmin: true } });
    const fetchSpy = mockFetchIssues();

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Old Movie")).toBeInTheDocument();
    });
    // Uploader column shown for the all-uploaders view.
    expect(screen.getByText("Uploader")).toBeInTheDocument();
    expect(screen.getByText("bob")).toBeInTheDocument();
    expect(String(fetchSpy.mock.calls[0][0])).toContain("scope=all");

    fireEvent.click(screen.getByRole("button", { name: "Only mine" }));

    await waitFor(() => {
      const last = fetchSpy.mock.calls[fetchSpy.mock.calls.length - 1][0];
      expect(String(last)).toContain("scope=mine");
    });
  });

  test("shows an empty state when nothing is missing", async () => {
    mockUseAuth.mockReturnValue({ user: { isAdmin: false } });
    vi.spyOn(globalThis, "fetch").mockResolvedValue({
      ok: true,
      json: async () => ({ issues: [], scope: "mine" }),
    } as Response);

    renderPage();

    await waitFor(() => {
      expect(
        screen.getByText(/No torrents are missing required metadata/i),
      ).toBeInTheDocument();
    });
  });
});
