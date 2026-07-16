import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { AdminInviteDistributionPage } from "@/pages/admin/AdminInviteDistributionPage";
import { ToastProvider } from "@/components/toast";

const mockFetch = vi.fn();
vi.stubGlobal("fetch", mockFetch);

interface FetchInit {
  method?: string;
  body?: string;
}

const groups = [
  { id: 5, name: "User", level: 20, is_admin: false, is_moderator: false },
  {
    id: 4,
    name: "Power User",
    level: 40,
    is_admin: false,
    is_moderator: false,
  },
  {
    id: 1,
    name: "Administrator",
    level: 100,
    is_admin: true,
    is_moderator: false,
  },
];

// Only the User class is eligible to start.
const rules = [
  {
    group_id: 5,
    min_ratio: 1,
    min_downloaded_bytes: 0,
    max_downloaded_bytes: 0,
    max_invites: 3,
  },
];

function mockApi() {
  const calls: { method: string; url: string; body?: string }[] = [];
  mockFetch.mockImplementation((url: string, init?: FetchInit) => {
    const method = init?.method ?? "GET";
    calls.push({ method, url, body: init?.body });
    if (method === "GET" && url.endsWith("/admin/groups")) {
      return Promise.resolve({ ok: true, json: async () => ({ groups }) });
    }
    if (method === "GET" && url.endsWith("/invite-distribution/rules")) {
      return Promise.resolve({ ok: true, json: async () => ({ rules }) });
    }
    if (url.endsWith("/invite-distribution/run")) {
      return Promise.resolve({
        ok: true,
        json: async () => ({ skipped: false, granted: 2 }),
      });
    }
    return Promise.resolve({ ok: true, json: async () => ({}) });
  });
  return calls;
}

function renderPage() {
  return render(
    <MemoryRouter>
      <ToastProvider>
        <AdminInviteDistributionPage />
      </ToastProvider>
    </MemoryRouter>,
  );
}

beforeEach(() => mockFetch.mockReset());
afterEach(() => {
  cleanup();
  mockFetch.mockReset();
});

describe("AdminInviteDistributionPage", () => {
  test("lists non-staff classes and excludes staff", async () => {
    mockApi();
    renderPage();
    expect(await screen.findByText("User")).toBeInTheDocument();
    expect(screen.getByText("Power User")).toBeInTheDocument();
    expect(screen.queryByText("Administrator")).not.toBeInTheDocument();
  });

  test("enables threshold inputs only for eligible classes", async () => {
    mockApi();
    renderPage();
    // User is eligible → editable.
    expect(await screen.findByLabelText("User Min Ratio")).toBeEnabled();
    // Power User is not configured yet → present but disabled until toggled on.
    expect(screen.getByLabelText("Power User Min Ratio")).toBeDisabled();
  });

  test("shows the save bar only after a change", async () => {
    mockApi();
    const user = userEvent.setup();
    renderPage();

    await screen.findByText("User");
    expect(
      screen.queryByRole("button", { name: "Save changes" }),
    ).not.toBeInTheDocument();

    await user.click(
      screen.getByLabelText("Include Power User in invite distribution"),
    );
    expect(
      await screen.findByRole("button", { name: "Save changes" }),
    ).toBeInTheDocument();
    expect(screen.getByText("1 unsaved change")).toBeInTheDocument();
  });

  test("batches adding a class and editing another into one save", async () => {
    const calls = mockApi();
    const user = userEvent.setup();
    renderPage();

    await screen.findByText("User");
    // Add Power User.
    await user.click(
      screen.getByLabelText("Include Power User in invite distribution"),
    );
    // Edit the User class threshold.
    const ratio = screen.getByLabelText("User Min Ratio");
    await user.clear(ratio);
    await user.type(ratio, "2");

    expect(screen.getByText("2 unsaved changes")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() => {
      expect(
        calls.filter(
          (c) =>
            c.method === "PUT" && c.url.includes("/invite-distribution/rules/"),
        ).length,
      ).toBe(2);
    });
    expect(
      calls.some((c) => c.method === "PUT" && c.url.endsWith("/rules/4")),
    ).toBe(true);
    expect(
      calls.some((c) => c.method === "PUT" && c.url.endsWith("/rules/5")),
    ).toBe(true);
  });

  test("removes a class with DELETE", async () => {
    const calls = mockApi();
    const user = userEvent.setup();
    renderPage();

    await screen.findByText("User");
    await user.click(
      screen.getByLabelText("Include User in invite distribution"),
    ); // toggle off
    await user.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() => {
      expect(
        calls.some(
          (c) =>
            c.method === "DELETE" &&
            c.url.endsWith("/invite-distribution/rules/5"),
        ),
      ).toBe(true);
    });
  });

  test("runs the engine on demand and reports the granted count", async () => {
    const calls = mockApi();
    const user = userEvent.setup();
    renderPage();

    await screen.findByText("User");
    await user.click(screen.getByRole("button", { name: "Run now" }));

    await waitFor(() => {
      expect(
        calls.some(
          (c) =>
            c.method === "POST" && c.url.endsWith("/invite-distribution/run"),
        ),
      ).toBe(true);
    });
  });

  test("reports a skip reason when the engine is disabled", async () => {
    mockFetch.mockImplementation((url: string, init?: FetchInit) => {
      const method = init?.method ?? "GET";
      if (method === "GET" && url.endsWith("/admin/groups")) {
        return Promise.resolve({ ok: true, json: async () => ({ groups }) });
      }
      if (method === "GET" && url.endsWith("/invite-distribution/rules")) {
        return Promise.resolve({ ok: true, json: async () => ({ rules }) });
      }
      if (url.endsWith("/invite-distribution/run")) {
        return Promise.resolve({
          ok: true,
          json: async () => ({ skipped: true, reason: "disabled" }),
        });
      }
      return Promise.resolve({ ok: true, json: async () => ({}) });
    });
    const user = userEvent.setup();
    renderPage();

    await screen.findByText("User");
    await user.click(screen.getByRole("button", { name: "Run now" }));

    expect(
      await screen.findByText(
        "Invite distribution is off — enable it in Site Settings to run",
      ),
    ).toBeInTheDocument();
  });
});
