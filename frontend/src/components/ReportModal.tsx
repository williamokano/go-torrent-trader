import { useState } from "react";
import { Modal } from "@/components/modal/Modal";
import { Textarea } from "@/components/form";
import "@/components/report-modal.css";

interface ReportModalProps {
  isOpen: boolean;
  onClose: () => void;
  torrentId: number;
  onSubmit: (torrentId: number, reason: string) => Promise<void>;
}

export function ReportModal({
  isOpen,
  onClose,
  torrentId,
  onSubmit,
}: ReportModalProps) {
  const [reason, setReason] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  function handleClose() {
    if (submitting) return;
    setReason("");
    setError(null);
    onClose();
  }

  async function handleSubmit() {
    const trimmed = reason.trim();
    if (!trimmed) {
      setError("Please provide a reason for this report.");
      return;
    }

    setSubmitting(true);
    setError(null);

    try {
      await onSubmit(torrentId, trimmed);
      setReason("");
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to submit report.");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Modal isOpen={isOpen} onClose={handleClose} title="Report Torrent">
      <div className="report-modal">
        <p className="report-modal__description">
          Please describe why you are reporting this torrent. Reports are
          reviewed by staff.
        </p>
        <Textarea
          label="Reason"
          value={reason}
          onChange={(e) => setReason(e.target.value)}
          rows={4}
          placeholder="Describe the issue..."
          error={error ?? undefined}
          disabled={submitting}
        />
        <div className="report-modal__actions">
          <button
            className="report-modal__cancel"
            onClick={handleClose}
            disabled={submitting}
            type="button"
          >
            Cancel
          </button>
          <button
            className="report-modal__submit"
            onClick={handleSubmit}
            disabled={submitting}
            type="button"
          >
            {submitting ? "Submitting..." : "Submit Report"}
          </button>
        </div>
      </div>
    </Modal>
  );
}
