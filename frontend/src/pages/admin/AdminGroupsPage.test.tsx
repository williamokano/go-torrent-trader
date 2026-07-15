import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { AdminGroupsPage } from "@/pages/admin/AdminGroupsPage";
import { ToastProvider } from "@/components/toast";

const mockFetch = vi.fn();
vi.stubGlobal("fetch", mockFetch);

interface FetchInit {
  method?: string;
  body?: string;
}

const vipGroup = {
  id: 7,
  name: "VIP",
  slug: "vip",
  level: 60,
  color: "#FFA500",
  can_upload: true,
  can_download: true,
  can_invite: true,
  can_comment: true,
  can_forum: true,
  is_admin: false,
  is_moderator: false,
  is_immune: false,
};

function mockApi() {
  const calls: { method: string; url: string; body?: string }[] = [];
  mockFetch.mockImplementation((url: string, init?: FetchInit) => {
    const method = init?.method ?? "GET";
    calls.push({ method, url, body: init?.body });
    if (method === "GET") {
      return Promise.resolve({
        ok: true,
        json: async () => ({ groups: [vipGroup] }),
      });
    }
    // POST / PUT / DELETE
    return Promise.resolve({ ok: true, json: async () => ({}) });
  });
  return calls;
}

function renderPage() {
  return render(
    <MemoryRouter>
      <ToastProvider>
        <AdminGroupsPage />
      </ToastProvider>
    </MemoryRouter>,
  );
}

afterEach(() => {
  cleanup();
  mockFetch.mockReset();
  vi.restoreAllMocks();
});

beforeEach(() => {
  mockFetch.mockReset();
});

describe("AdminGroupsPage", () => {
  test("lists groups from the API", async () => {
    mockApi();
    renderPage();
    expect(await screen.findByText("VIP")).toBeInTheDocument();
  });

  test("creates a group through the New Group modal", async () => {
    const calls = mockApi();
    const user = userEvent.setup();
    renderPage();

    await screen.findByText("VIP");
    await user.click(screen.getByRole("button", { name: "New Group" }));

    const dialog = await screen.findByRole("dialog");
    await user.type(within(dialog).getByLabelText("Name"), "Seedbox");
    await user.click(within(dialog).getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(
        calls.some(
          (c) => c.method === "POST" && c.url.endsWith("/admin/groups"),
        ),
      ).toBe(true);
    });
    const post = calls.find((c) => c.method === "POST");
    expect(JSON.parse(post!.body!)).toMatchObject({ name: "Seedbox" });
  });

  test("deletes a group after confirmation", async () => {
    vi.spyOn(window, "confirm").mockReturnValue(true);
    const calls = mockApi();
    const user = userEvent.setup();
    renderPage();

    await screen.findByText("VIP");
    await user.click(screen.getByRole("button", { name: "Delete" }));

    await waitFor(() => {
      expect(
        calls.some(
          (c) => c.method === "DELETE" && c.url.endsWith("/admin/groups/7"),
        ),
      ).toBe(true);
    });
  });

  test("does not delete when confirmation is declined", async () => {
    vi.spyOn(window, "confirm").mockReturnValue(false);
    const calls = mockApi();
    const user = userEvent.setup();
    renderPage();

    await screen.findByText("VIP");
    await user.click(screen.getByRole("button", { name: "Delete" }));

    // Give any erroneous request a chance to fire.
    await new Promise((r) => setTimeout(r, 0));
    expect(calls.some((c) => c.method === "DELETE")).toBe(false);
  });
});
