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
    expect(
      screen.getByText(/no automatic deletion runs yet/i),
    ).toBeInTheDocument();

    // The raw key must no longer surface as a label.
    expect(
      screen.queryByText("cheat_detection_enabled"),
    ).not.toBeInTheDocument();
  });
});
