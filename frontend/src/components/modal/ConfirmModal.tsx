import { Modal } from "./Modal";
import "@/components/modal/modal.css";

interface ConfirmModalProps {
  isOpen: boolean;
  title: string;
  message: string;
  confirmLabel?: string;
  cancelLabel?: string;
  danger?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}

export function ConfirmModal({
  isOpen,
  title,
  message,
  confirmLabel = "Confirm",
  cancelLabel = "Cancel",
  danger = false,
  onConfirm,
  onCancel,
}: ConfirmModalProps) {
  return (
    <Modal isOpen={isOpen} onClose={onCancel} title={title}>
      <div className="modal-body">
        <p>{message}</p>
      </div>
      <div className="modal-footer">
        <button className="modal-btn modal-btn--secondary" onClick={onCancel}>
          {cancelLabel}
        </button>
        <button
          className={`modal-btn ${danger ? "modal-btn--danger" : "modal-btn--primary"}`}
          onClick={onConfirm}
        >
          {confirmLabel}
        </button>
      </div>
    </Modal>
  );
}
