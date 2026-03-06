import {
  cleanup,
  render,
  screen,
  fireEvent,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, test, expect, vi } from "vitest";
import { ReportModal } from "@/components/ReportModal";

afterEach(() => {
  cleanup();
  document.body.innerHTML = "";
});

function renderModal(
  overrides: Partial<Parameters<typeof ReportModal>[0]> = {},
) {
  const defaultProps = {
    isOpen: true,
    onClose: vi.fn(),
    torrentId: 42,
    onSubmit: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  };
  const result = render(<ReportModal {...defaultProps} />);
  return { ...result, props: defaultProps };
}

describe("ReportModal", () => {
  test("renders modal with title and description when open", () => {
    renderModal();
    expect(screen.getByText("Report Torrent")).toBeInTheDocument();
    expect(
      screen.getByText(/Please describe why you are reporting/),
    ).toBeInTheDocument();
    expect(screen.getByLabelText("Reason")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Submit Report" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Cancel" })).toBeInTheDocument();
  });

  test("does not render when isOpen is false", () => {
    renderModal({ isOpen: false });
    expect(screen.queryByText("Report Torrent")).not.toBeInTheDocument();
  });

  test("shows validation error when submitting empty reason", async () => {
    const { props } = renderModal();

    fireEvent.click(screen.getByRole("button", { name: "Submit Report" }));

    await waitFor(() => {
      expect(
        screen.getByText("Please provide a reason for this report."),
      ).toBeInTheDocument();
    });

    expect(props.onSubmit).not.toHaveBeenCalled();
  });

  test("shows validation error for whitespace-only reason", async () => {
    const { props } = renderModal();

    fireEvent.change(screen.getByLabelText("Reason"), {
      target: { value: "   " },
    });
    fireEvent.click(screen.getByRole("button", { name: "Submit Report" }));

    await waitFor(() => {
      expect(
        screen.getByText("Please provide a reason for this report."),
      ).toBeInTheDocument();
    });

    expect(props.onSubmit).not.toHaveBeenCalled();
  });

  test("calls onSubmit with torrentId and trimmed reason", async () => {
    const { props } = renderModal();

    fireEvent.change(screen.getByLabelText("Reason"), {
      target: { value: "  Fake content  " },
    });
    fireEvent.click(screen.getByRole("button", { name: "Submit Report" }));

    await waitFor(() => {
      expect(props.onSubmit).toHaveBeenCalledWith(42, "Fake content");
    });
  });

  test("calls onClose after successful submission", async () => {
    const { props } = renderModal();

    fireEvent.change(screen.getByLabelText("Reason"), {
      target: { value: "Copyright violation" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Submit Report" }));

    await waitFor(() => {
      expect(props.onClose).toHaveBeenCalled();
    });
  });

  test("shows error message when onSubmit rejects", async () => {
    const onSubmit = vi.fn().mockRejectedValue(new Error("Server error"));
    renderModal({ onSubmit });

    fireEvent.change(screen.getByLabelText("Reason"), {
      target: { value: "Bad torrent" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Submit Report" }));

    await waitFor(() => {
      expect(screen.getByText("Server error")).toBeInTheDocument();
    });
  });

  test("shows generic error for non-Error rejection", async () => {
    const onSubmit = vi.fn().mockRejectedValue("unknown");
    renderModal({ onSubmit });

    fireEvent.change(screen.getByLabelText("Reason"), {
      target: { value: "Bad torrent" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Submit Report" }));

    await waitFor(() => {
      expect(screen.getByText("Failed to submit report.")).toBeInTheDocument();
    });
  });

  test("disables buttons while submitting", async () => {
    let resolveSubmit: () => void;
    const onSubmit = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          resolveSubmit = resolve;
        }),
    );
    renderModal({ onSubmit });

    fireEvent.change(screen.getByLabelText("Reason"), {
      target: { value: "Spam content" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Submit Report" }));

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "Submitting..." }),
      ).toBeDisabled();
    });
    expect(screen.getByRole("button", { name: "Cancel" })).toBeDisabled();

    resolveSubmit!();

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalled();
    });
  });

  test("cancel button calls onClose and resets state", () => {
    const { props } = renderModal();

    fireEvent.change(screen.getByLabelText("Reason"), {
      target: { value: "Some reason" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(props.onClose).toHaveBeenCalled();
  });
});
