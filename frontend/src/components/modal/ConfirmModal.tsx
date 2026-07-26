import { Modal } from "./Modal";
import "@/components/modal/modal.css";

interface ConfirmModalProps {
  isOpen: boolean;
  title: string;
  message: string;
  confirmLabel?: string;
  cancelLabel?: string;
  /**
   * Shown on the confirm button while `loading`. Defaults to "Deleting..." because
   * that is what this modal used to be hardcoded to and every existing caller
   * deletes — but it now also drives ban and unban, where a button reading
   * "Deleting..." is the copy most likely to make someone panic-cancel.
   */
  loadingLabel?: string;
  danger?: boolean;
  loading?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}

export function ConfirmModal({
  isOpen,
  title,
  message,
  confirmLabel = "Confirm",
  cancelLabel = "Cancel",
  loadingLabel = "Deleting...",
  danger = false,
  loading = false,
  onConfirm,
  onCancel,
}: ConfirmModalProps) {
  return (
    <Modal isOpen={isOpen} onClose={onCancel} title={title}>
      <div className="modal-body">
        <p>{message}</p>
      </div>
      <div className="modal-footer">
        <button
          className="modal-btn modal-btn--secondary"
          onClick={onCancel}
          disabled={loading}
        >
          {cancelLabel}
        </button>
        <button
          className={`modal-btn ${danger ? "modal-btn--danger" : "modal-btn--primary"}`}
          onClick={onConfirm}
          disabled={loading}
        >
          {loading ? loadingLabel : confirmLabel}
        </button>
      </div>
    </Modal>
  );
}
