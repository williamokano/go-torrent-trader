import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, test, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { AdminSettingsPage } from "@/pages/admin/AdminSettingsPage";
import { ToastProvider } from "@/components/toast";

const mockFetch = vi.fn();
vi.stubGlobal("fetch", mockFetch);

function renderPage() {
  return render(
    <MemoryRouter>
      <ToastProvider>
        <AdminSettingsPage />
      </ToastProvider>
    </MemoryRouter>,
  );
}

afterEach(() => {
  cleanup();
  mockFetch.mockReset();
});

describe("AdminSettingsPage", () => {
  test("renders friendly labels and descriptions for cheat and announce-log settings", async () => {
    const now = new Date().toISOString();
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => ({
        settings: [
          { key: "cheat_detection_enabled", value: "true", updated_at: now },
          { key: "announce_log_retention_days", value: "90", updated_at: now },
        ],
      }),
    });

    renderPage();

    // Previously these rendered as their raw keys; now they resolve to labels.
    expect(
      await screen.findByText("Cheat Detection Enabled"),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        /each announce is checked for impossible upload speeds/i,
      ),
    ).toBeInTheDocument();

    expect(
      await screen.findByText("Announce Log Retention (days)"),
    ).toBeInTheDocument();
    // The description has to state that the window now deletes, and that the
    // monthly totals survive it — an operator shortening this setting is deciding
    // how long to keep IP addresses, and needs to know what it costs.
    expect(
      screen.getByText(/deletes raw rows past this window/i),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/without losing anyone's transfer totals/i),
    ).toBeInTheDocument();

    // The raw key must no longer surface as a label.
    expect(
      screen.queryByText("cheat_detection_enabled"),
    ).not.toBeInTheDocument();
  });

  // The page renders whatever rows the API returns, so a setting without a
  // definition here shows up as its bare key — which is how an operator ends up
  // unable to tell what "chat_system_display_name" does.
  test("labels the shoutbox system name setting", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => ({
        settings: [
          {
            key: "chat_system_display_name",
            value: "System",
            updated_at: new Date().toISOString(),
          },
        ],
      }),
    });

    renderPage();

    expect(await screen.findByText("Shoutbox System Name")).toBeInTheDocument();
    expect(
      screen.getByText(/author label shown on shoutbox announcements/i),
    ).toBeInTheDocument();
    expect(screen.getByDisplayValue("System")).toBeInTheDocument();
    expect(
      screen.queryByText("chat_system_display_name"),
    ).not.toBeInTheDocument();
  });
});
