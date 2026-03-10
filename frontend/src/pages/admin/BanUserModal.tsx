import { useState } from "react";
import { Modal } from "@/components/modal/Modal";
import { Textarea, Input, Checkbox } from "@/components/form";
import "@/components/modal/modal.css";

interface BanUserModalProps {
  isOpen: boolean;
  username: string;
  email?: string;
  onConfirm: (data: {
    reason: string;
    ban_ip: boolean;
    ban_email: boolean;
    duration_days: number | null;
  }) => void;
  onCancel: () => void;
  loading?: boolean;
}

/**
 * Inner component that mounts/unmounts based on isOpen,
 * so state is naturally reset when the modal closes and reopens.
 */
function BanUserModalContent({
  username,
  email,
  onConfirm,
  onCancel,
  loading = false,
}: Omit<BanUserModalProps, "isOpen">) {
  const [reason, setReason] = useState("");
  const [banIP, setBanIP] = useState(false);
  const [banEmail, setBanEmail] = useState(false);
  const [durationDays, setDurationDays] = useState("");

  const emailDomain = email?.split("@")[1] || "";
  const emailPattern = emailDomain ? `*@${emailDomain}` : "";

  const handleConfirm = () => {
    if (!reason.trim()) return;
    onConfirm({
      reason: reason.trim(),
      ban_ip: banIP,
      ban_email: banEmail,
      duration_days: durationDays ? parseInt(durationDays, 10) : null,
    });
  };

  return (
    <Modal isOpen={true} onClose={onCancel} title={`Ban ${username}`}>
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
            label={
              banEmail && emailPattern
                ? `Also ban email domain (will ban ${emailPattern})`
                : "Also ban email domain"
            }
            checked={banEmail}
            onChange={(e) => setBanEmail(e.target.checked)}
          />
          {banEmail && emailPattern && (
            <p
              style={{
                margin: "var(--space-xs) 0 0 var(--space-lg)",
                fontSize: "0.85em",
                color: "var(--color-warning, #e67e22)",
              }}
            >
              This will block all future registrations from {emailPattern}
            </p>
          )}
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
          onClick={onCancel}
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

export function BanUserModal({ isOpen, ...rest }: BanUserModalProps) {
  if (!isOpen) return null;
  return <BanUserModalContent {...rest} />;
}
