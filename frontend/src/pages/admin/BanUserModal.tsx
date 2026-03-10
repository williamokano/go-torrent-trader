import { useState } from "react";
import { Modal } from "@/components/modal/Modal";
import { Textarea, Input, Checkbox } from "@/components/form";
import "@/components/modal/modal.css";

interface BanUserModalProps {
  isOpen: boolean;
  username: string;
  onConfirm: (data: {
    reason: string;
    ban_ip: boolean;
    ban_email: boolean;
    duration_days: number | null;
  }) => void;
  onCancel: () => void;
  loading?: boolean;
}

export function BanUserModal({
  isOpen,
  username,
  onConfirm,
  onCancel,
  loading = false,
}: BanUserModalProps) {
  const [reason, setReason] = useState("");
  const [banIP, setBanIP] = useState(false);
  const [banEmail, setBanEmail] = useState(false);
  const [durationDays, setDurationDays] = useState("");

  const handleConfirm = () => {
    if (!reason.trim()) return;
    onConfirm({
      reason: reason.trim(),
      ban_ip: banIP,
      ban_email: banEmail,
      duration_days: durationDays ? parseInt(durationDays, 10) : null,
    });
  };

  const handleCancel = () => {
    setReason("");
    setBanIP(false);
    setBanEmail(false);
    setDurationDays("");
    onCancel();
  };

  return (
    <Modal isOpen={isOpen} onClose={handleCancel} title={`Ban ${username}`}>
      <div className="modal-body">
        <Textarea
          label="Reason (required)"
          placeholder="Why is this user being banned?"
          value={reason}
          onChange={(e) => setReason(e.target.value)}
          rows={3}
        />
        <div style={{ marginTop: "var(--space-md)" }}>
          <Checkbox
            label="Also ban IP address"
            checked={banIP}
            onChange={(e) => setBanIP(e.target.checked)}
          />
        </div>
        <div style={{ marginTop: "var(--space-sm)" }}>
          <Checkbox
            label="Also ban email domain"
            checked={banEmail}
            onChange={(e) => setBanEmail(e.target.checked)}
          />
        </div>
        <div style={{ marginTop: "var(--space-md)" }}>
          <Input
            label="Duration (days, empty = permanent)"
            type="number"
            min="1"
            placeholder="Leave empty for permanent ban"
            value={durationDays}
            onChange={(e) => setDurationDays(e.target.value)}
          />
        </div>
      </div>
      <div className="modal-footer">
        <button
          className="modal-btn modal-btn--secondary"
          onClick={handleCancel}
          disabled={loading}
        >
          Cancel
        </button>
        <button
          className="modal-btn modal-btn--danger"
          onClick={handleConfirm}
          disabled={!reason.trim() || loading}
        >
          {loading ? "Banning..." : "Ban User"}
        </button>
      </div>
    </Modal>
  );
}
