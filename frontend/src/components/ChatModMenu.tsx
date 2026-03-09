import { useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { useChat } from "@/lib/useChat";
import { ConfirmModal } from "@/components/modal";
import "./chat-mod-menu.css";

interface ChatModMenuProps {
  userId: number;
  username: string;
}

export function ChatModMenu({ userId, username }: ChatModMenuProps) {
  const { deleteUserMessages, muteUser, unmuteUser } = useChat();
  const [open, setOpen] = useState(false);
  const [showMuteForm, setShowMuteForm] = useState(false);
  const [confirmDeleteAll, setConfirmDeleteAll] = useState(false);
  const [muteDuration, setMuteDuration] = useState("10");
  const [muteReason, setMuteReason] = useState("");
  const menuRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const [dropdownStyle, setDropdownStyle] = useState<React.CSSProperties>({});

  // Position dropdown above trigger using fixed positioning to escape overflow containers
  useEffect(() => {
    if (!open || !triggerRef.current) return;
    const rect = triggerRef.current.getBoundingClientRect();
    setDropdownStyle({
      position: "fixed",
      left: rect.left,
      bottom: window.innerHeight - rect.top + 4,
      zIndex: 1000,
    });
  }, [open]);

  // Close menu on outside click or Escape
  useEffect(() => {
    if (!open) return;
    const handleClick = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setOpen(false);
        setShowMuteForm(false);
      }
    };
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        setOpen(false);
        setShowMuteForm(false);
      }
    };
    document.addEventListener("mousedown", handleClick);
    document.addEventListener("keydown", handleKeyDown);
    return () => {
      document.removeEventListener("mousedown", handleClick);
      document.removeEventListener("keydown", handleKeyDown);
    };
  }, [open]);

  const handleDeleteAll = useCallback(async () => {
    await deleteUserMessages(userId);
    setOpen(false);
    setConfirmDeleteAll(false);
  }, [userId, deleteUserMessages]);

  const handleMute = useCallback(async () => {
    const duration = parseInt(muteDuration, 10);
    if (!duration || duration <= 0) return;
    await muteUser(userId, duration, muteReason);
    setOpen(false);
    setShowMuteForm(false);
    setMuteDuration("10");
    setMuteReason("");
  }, [userId, muteDuration, muteReason, muteUser]);

  const handleUnmute = useCallback(async () => {
    await unmuteUser(userId);
    setOpen(false);
  }, [userId, unmuteUser]);

  return (
    <div className="chat-mod-menu" ref={menuRef}>
      <button
        ref={triggerRef}
        className="chat-mod-menu__trigger"
        onClick={() => {
          setOpen((p) => !p);
          setShowMuteForm(false);
        }}
        title="Moderation actions"
      >
        {username}
      </button>
      {open && (
        <div className="chat-mod-menu__dropdown" style={dropdownStyle}>
          <Link
            to={`/user/${userId}`}
            className="chat-mod-menu__item"
            onClick={() => setOpen(false)}
          >
            View profile
          </Link>
          <button className="chat-mod-menu__item" onClick={() => setConfirmDeleteAll(true)}>
            Delete all messages
          </button>
          <button
            className="chat-mod-menu__item"
            onClick={() => setShowMuteForm((p) => !p)}
          >
            Mute user
          </button>
          <button className="chat-mod-menu__item" onClick={handleUnmute}>
            Unmute user
          </button>
          {showMuteForm && (
            <div className="chat-mod-menu__mute-form">
              <label className="chat-mod-menu__mute-label">
                Duration (minutes)
              </label>
              <input
                type="number"
                min="1"
                max="43200"
                value={muteDuration}
                onChange={(e) => setMuteDuration(e.target.value)}
                placeholder="e.g. 10"
                title="Mute duration in minutes (max 30 days)"
                className="chat-mod-menu__mute-input"
              />
              <input
                type="text"
                value={muteReason}
                onChange={(e) => setMuteReason(e.target.value)}
                placeholder="Reason (optional)"
                className="chat-mod-menu__mute-input"
              />
              <button className="chat-mod-menu__mute-btn" onClick={handleMute}>
                Confirm mute
              </button>
            </div>
          )}
        </div>
      )}
      <ConfirmModal
        isOpen={confirmDeleteAll}
        title="Delete All Messages"
        message={`Delete all chat messages from ${username}?`}
        confirmLabel="Delete All"
        danger
        onConfirm={handleDeleteAll}
        onCancel={() => setConfirmDeleteAll(false)}
      />
    </div>
  );
}
