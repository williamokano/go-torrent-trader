import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, cleanup } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import {
  TorrentModerationPanel,
  ApprovedByLine,
} from "./TorrentModerationPanel";
import type { TorrentModeration } from "@/types/moderation";

vi.mock("@/features/auth/token", () => ({
  getAccessToken: () => "test-token",
}));
vi.mock("@/config", () => ({
  getConfig: () => ({ API_URL: "http://localhost:8080" }),
}));
const mockToast = { success: vi.fn(), error: vi.fn() };
vi.mock("@/components/toast", () => ({
  useToast: () => mockToast,
}));

function renderPanel(
  moderation: TorrentModeration,
  opts: {
    isStaff?: boolean;
    canApprove?: boolean;
    onChanged?: () => void;
  } = {},
) {
  return render(
    <MemoryRouter>
      <TorrentModerationPanel
        torrentId={7}
        moderation={moderation}
        isStaff={opts.isStaff ?? false}
        canApprove={opts.canApprove ?? false}
        onChanged={opts.onChanged ?? (() => {})}
      />
    </MemoryRouter>,
  );
}

describe("TorrentModerationPanel", () => {
  beforeEach(() => {
    cleanup();
    vi.restoreAllMocks();
    mockToast.success.mockClear();
    mockToast.error.mockClear();
  });

  it("shows staff actions on a pending torrent and approves", async () => {
    const user = userEvent.setup();
    const onChanged = vi.fn();
    const fetchSpy = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue({ ok: true, json: async () => ({}) } as Response);

    renderPanel(
      { status: "pending" },
      { isStaff: true, canApprove: true, onChanged },
    );

    expect(screen.getByText("Awaiting moderation")).toBeInTheDocument();
    expect(screen.getByText("Unassigned")).toBeInTheDocument();
    expect(screen.getByText("Claim")).toBeInTheDocument();
    expect(screen.getByText("Reject")).toBeInTheDocument();

    await user.click(screen.getByText("Approve"));

    await waitFor(() => {
      expect(fetchSpy).toHaveBeenCalledWith(
        "http://localhost:8080/api/v1/torrents/7/moderation/approve",
        expect.objectContaining({ method: "POST" }),
      );
    });
    expect(onChanged).toHaveBeenCalled();
    expect(mockToast.success).toHaveBeenCalledWith("Torrent approved");
  });

  it("hides staff actions from a non-staff author", () => {
    renderPanel({ status: "pending" }, { isStaff: false, canApprove: false });
    expect(screen.queryByText("Approve")).not.toBeInTheDocument();
    expect(screen.queryByText("Reject")).not.toBeInTheDocument();
    expect(screen.queryByText("Claim")).not.toBeInTheDocument();
  });

  it("shows a rejected note", () => {
    renderPanel({ status: "rejected" }, { isStaff: true });
    expect(screen.getByText("Rejected")).toBeInTheDocument();
    expect(screen.getByText(/was rejected/i)).toBeInTheDocument();
  });
});

describe("ApprovedByLine", () => {
  it("renders the approver when approved", () => {
    render(
      <MemoryRouter>
        <ApprovedByLine
          moderation={{
            status: "approved",
            approved_by_id: 3,
            approved_by_name: "carol",
          }}
        />
      </MemoryRouter>,
    );
    expect(screen.getByText("Approved by")).toBeInTheDocument();
    expect(screen.getByText("carol")).toBeInTheDocument();
  });

  it("renders nothing without an approver", () => {
    const { container } = render(
      <MemoryRouter>
        <ApprovedByLine moderation={{ status: "approved" }} />
      </MemoryRouter>,
    );
    expect(container).toBeEmptyDOMElement();
  });
});
