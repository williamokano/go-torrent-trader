import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { AdminBansPage } from "@/pages/admin/AdminBansPage";
import { ToastProvider } from "@/components/toast";

const mockFetch = vi.fn();

vi.stubGlobal("fetch", mockFetch);

afterEach(cleanup);

beforeEach(() => {
  vi.clearAllMocks();
});

function renderPage() {
  return render(
    <ToastProvider>
      <MemoryRouter initialEntries={["/admin/bans"]}>
        <AdminBansPage />
      </MemoryRouter>
    </ToastProvider>,
  );
}

describe("AdminBansPage", () => {
  test("renders page title and section headers", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => ({ email_bans: [], ip_bans: [] }),
    });

    renderPage();

    expect(screen.getByText("Bans")).toBeInTheDocument();
    expect(screen.getByText("Email Bans")).toBeInTheDocument();
    expect(screen.getByText("IP Bans")).toBeInTheDocument();
  });

  test("displays email bans from API", async () => {
    mockFetch.mockImplementation((url: string) => {
      if (url.includes("/bans/emails")) {
        return Promise.resolve({
          ok: true,
          json: async () => ({
            email_bans: [
              {
                id: 1,
                pattern: "%@mailinator.com",
                reason: "Disposable email",
                created_by: 1,
                created_at: new Date().toISOString(),
              },
            ],
          }),
        });
      }
      return Promise.resolve({
        ok: true,
        json: async () => ({ ip_bans: [] }),
      });
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("%@mailinator.com")).toBeInTheDocument();
    });
    expect(screen.getByText("Disposable email")).toBeInTheDocument();
  });

  test("displays IP bans from API", async () => {
    mockFetch.mockImplementation((url: string) => {
      if (url.includes("/bans/ips")) {
        return Promise.resolve({
          ok: true,
          json: async () => ({
            ip_bans: [
              {
                id: 1,
                ip_range: "10.0.0.0/8",
                reason: "Known VPN",
                created_by: 1,
                created_at: new Date().toISOString(),
              },
            ],
          }),
        });
      }
      return Promise.resolve({
        ok: true,
        json: async () => ({ email_bans: [] }),
      });
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("10.0.0.0/8")).toBeInTheDocument();
    });
    expect(screen.getByText("Known VPN")).toBeInTheDocument();
  });

  test("shows empty state when no bans exist", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => ({ email_bans: [], ip_bans: [] }),
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("No email bans configured.")).toBeInTheDocument();
      expect(screen.getByText("No IP bans configured.")).toBeInTheDocument();
    });
  });

  test("renders add email ban form with inputs", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => ({ email_bans: [], ip_bans: [] }),
    });

    renderPage();

    expect(screen.getByLabelText("Pattern")).toBeInTheDocument();
    // Two "Reason (optional)" labels exist (one for email, one for IP)
    const reasonLabels = screen.getAllByText("Reason (optional)");
    expect(reasonLabels.length).toBe(2);
  });

  test("renders add IP ban form with inputs", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => ({ email_bans: [], ip_bans: [] }),
    });

    renderPage();

    expect(screen.getByLabelText("IP / CIDR Range")).toBeInTheDocument();
  });
});
