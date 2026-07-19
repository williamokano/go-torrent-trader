import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { userEvent } from "@testing-library/user-event";
import { afterEach, beforeEach, describe, test, expect, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { ToastProvider } from "@/components/toast";
import { NotificationsPage } from "@/pages/NotificationsPage";

vi.mock("@/features/auth/token", () => ({
  getAccessToken: () => "fake-token",
}));

vi.mock("@/config", () => ({
  getConfig: () => ({ API_URL: "http://localhost:8080", SITE_NAME: "Test" }),
}));

vi.mock("@/lib/useChat", () => ({
  useChat: () => ({
    setNotifUnreadCount: vi.fn(),
  }),
}));

const FAKE_PREFERENCES = {
  preferences: [
    { notification_type: "forum_reply", enabled: true },
    { notification_type: "mention", enabled: true },
    { notification_type: "pm_received", enabled: false },
  ],
};

const mockFetch = vi.fn();

afterEach(() => {
  cleanup();
});

beforeEach(() => {
  vi.clearAllMocks();
  mockFetch.mockImplementation((url: string) => {
    if (url.includes("/digest-preference")) {
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve({ frequency: "off" }),
      });
    }
    if (url.includes("/notifications/preferences")) {
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve(FAKE_PREFERENCES),
      });
    }
    return Promise.resolve({
      ok: true,
      json: () =>
        Promise.resolve({ notifications: [], total: 0, page: 1, per_page: 25 }),
    });
  });
  vi.stubGlobal("fetch", mockFetch);
});

function renderNotificationsPage() {
  return render(
    <MemoryRouter>
      <ToastProvider>
        <NotificationsPage />
      </ToastProvider>
    </MemoryRouter>,
  );
}

describe("NotificationsPage digest preference", () => {
  test("fetches and displays the current digest frequency when the Preferences tab opens", async () => {
    mockFetch.mockImplementation((url: string) => {
      if (url.includes("/digest-preference")) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ frequency: "weekly" }),
        });
      }
      if (url.includes("/notifications/preferences")) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve(FAKE_PREFERENCES),
        });
      }
      return Promise.resolve({
        ok: true,
        json: () =>
          Promise.resolve({
            notifications: [],
            total: 0,
            page: 1,
            per_page: 25,
          }),
      });
    });

    const user = userEvent.setup();
    renderNotificationsPage();

    await user.click(screen.getByText("Preferences"));

    await waitFor(() => {
      expect(screen.getByLabelText("Email digest")).toBeInTheDocument();
    });
    expect(screen.getByLabelText("Email digest")).toHaveValue("weekly");
  });

  test("defaults to off when no digest preference has been saved", async () => {
    const user = userEvent.setup();
    renderNotificationsPage();

    await user.click(screen.getByText("Preferences"));

    await waitFor(() => {
      expect(screen.getByLabelText("Email digest")).toHaveValue("off");
    });
  });

  test("changing the digest select PUTs the new frequency", async () => {
    const user = userEvent.setup();
    renderNotificationsPage();

    await user.click(screen.getByText("Preferences"));
    await waitFor(() => {
      expect(screen.getByLabelText("Email digest")).toBeInTheDocument();
    });

    await user.selectOptions(screen.getByLabelText("Email digest"), "daily");

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining("/api/v1/notifications/digest-preference"),
        expect.objectContaining({
          method: "PUT",
          body: JSON.stringify({ frequency: "daily" }),
        }),
      );
    });
    expect(screen.getByLabelText("Email digest")).toHaveValue("daily");
  });

  test("reverts the select and shows an error toast when the update fails", async () => {
    mockFetch.mockImplementation((url: string, init?: RequestInit) => {
      if (url.includes("/digest-preference") && init?.method === "PUT") {
        return Promise.resolve({ ok: false, json: () => Promise.resolve({}) });
      }
      if (url.includes("/digest-preference")) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ frequency: "off" }),
        });
      }
      if (url.includes("/notifications/preferences")) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve(FAKE_PREFERENCES),
        });
      }
      return Promise.resolve({
        ok: true,
        json: () =>
          Promise.resolve({
            notifications: [],
            total: 0,
            page: 1,
            per_page: 25,
          }),
      });
    });

    const user = userEvent.setup();
    renderNotificationsPage();

    await user.click(screen.getByText("Preferences"));
    await waitFor(() => {
      expect(screen.getByLabelText("Email digest")).toHaveValue("off");
    });

    await user.selectOptions(screen.getByLabelText("Email digest"), "weekly");

    await waitFor(() => {
      expect(
        screen.getByText("Failed to update digest preference"),
      ).toBeInTheDocument();
    });
    expect(screen.getByLabelText("Email digest")).toHaveValue("off");
  });

  test("still renders the per-type preference toggles alongside the digest control", async () => {
    const user = userEvent.setup();
    renderNotificationsPage();

    await user.click(screen.getByText("Preferences"));

    await waitFor(() => {
      expect(screen.getByText("Mention")).toBeInTheDocument();
    });
    expect(screen.getByText("Private Message")).toBeInTheDocument();
    expect(screen.getByLabelText("Email digest")).toBeInTheDocument();
  });
});
