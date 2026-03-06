import { useCallback, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Input, Select, Textarea, Checkbox } from "@/components/form";
import { useToast } from "@/components/toast";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";
import "./upload.css";

const CATEGORIES = [
  { value: "", label: "Select a category" },
  { value: "1", label: "Movies" },
  { value: "2", label: "TV" },
  { value: "3", label: "Music" },
  { value: "4", label: "Games" },
  { value: "5", label: "Software" },
  { value: "6", label: "Anime" },
  { value: "7", label: "Books" },
  { value: "8", label: "Other" },
];

export function UploadPage() {
  const toast = useToast();
  const navigate = useNavigate();
  const fileInputRef = useRef<HTMLInputElement>(null);

  const [torrentFile, setTorrentFile] = useState<File | null>(null);
  const [categoryId, setCategoryId] = useState("");
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [anonymous, setAnonymous] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [isDragOver, setIsDragOver] = useState(false);
  const [fileError, setFileError] = useState<string | null>(null);

  const handleFile = useCallback((file: File) => {
    if (!file.name.endsWith(".torrent")) {
      setFileError("Please select a .torrent file");
      setTorrentFile(null);
      return;
    }
    setFileError(null);
    setTorrentFile(file);
    // Auto-fill name from filename (strip .torrent extension)
    setName((prev) => (prev ? prev : file.name.replace(/\.torrent$/, "")));
  }, []);

  function handleFileChange(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (file) handleFile(file);
  }

  function handleDrop(e: React.DragEvent) {
    e.preventDefault();
    setIsDragOver(false);
    const file = e.dataTransfer.files?.[0];
    if (file) handleFile(file);
  }

  function handleDragOver(e: React.DragEvent) {
    e.preventDefault();
    setIsDragOver(true);
  }

  function handleDragLeave(e: React.DragEvent) {
    e.preventDefault();
    setIsDragOver(false);
  }

  function handleDropzoneClick() {
    fileInputRef.current?.click();
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();

    if (!torrentFile) {
      setFileError("A .torrent file is required");
      return;
    }
    if (!categoryId) {
      toast.error("Please select a category");
      return;
    }

    setIsSubmitting(true);

    try {
      const token = getAccessToken();
      if (!token) {
        toast.error("You must be logged in to upload");
        return;
      }

      const formData = new FormData();
      formData.append("torrent_file", torrentFile);
      formData.append("category_id", categoryId);
      if (name.trim()) formData.append("name", name.trim());
      if (description.trim())
        formData.append("description", description.trim());
      if (anonymous) formData.append("anonymous", "true");

      const response = await fetch(`${getConfig().API_URL}/api/v1/torrents`, {
        method: "POST",
        headers: {
          Authorization: `Bearer ${token}`,
        },
        body: formData,
      });

      if (!response.ok) {
        const body = await response.json().catch(() => null);
        const message =
          body?.error?.message ?? `Upload failed (${response.status})`;
        throw new Error(message);
      }

      const data = await response.json();
      toast.success("Torrent uploaded successfully!");

      const torrentId = data?.torrent?.id;
      if (torrentId) {
        navigate(`/torrent/${torrentId}`);
      } else {
        navigate("/browse");
      }
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : "Upload failed. Please try again.",
      );
    } finally {
      setIsSubmitting(false);
    }
  }

  const dropzoneClass = [
    "upload-dropzone",
    isDragOver ? "upload-dropzone--active" : "",
    fileError ? "upload-dropzone--error" : "",
  ]
    .filter(Boolean)
    .join(" ");

  return (
    <div className="upload-page">
      <div className="upload-card">
        <h1 className="upload-card__title">Upload Torrent</h1>
        <form className="upload-card__form" onSubmit={handleSubmit}>
          {/* File drop zone */}
          <div
            className={dropzoneClass}
            onClick={handleDropzoneClick}
            onDrop={handleDrop}
            onDragOver={handleDragOver}
            onDragLeave={handleDragLeave}
            role="button"
            tabIndex={0}
            onKeyDown={(e) => {
              if (e.key === "Enter" || e.key === " ") handleDropzoneClick();
            }}
            aria-label="Select torrent file"
          >
            <input
              ref={fileInputRef}
              type="file"
              accept=".torrent"
              onChange={handleFileChange}
              style={{ display: "none" }}
              data-testid="file-input"
            />
            {torrentFile ? (
              <span className="upload-dropzone__filename">
                {torrentFile.name}
              </span>
            ) : (
              <>
                <span className="upload-dropzone__label">
                  Drop .torrent file here or click to browse
                </span>
                <span className="upload-dropzone__hint">
                  Only .torrent files are accepted
                </span>
              </>
            )}
            {fileError && (
              <span className="upload-dropzone__error">{fileError}</span>
            )}
          </div>

          <Select
            label="Category"
            options={CATEGORIES}
            value={categoryId}
            onChange={(e) => setCategoryId(e.target.value)}
          />

          <Input
            label="Name"
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Auto-filled from torrent file"
          />

          <Textarea
            label="Description"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            rows={5}
            placeholder="Describe the torrent contents..."
          />

          <Checkbox
            label="Upload anonymously"
            checked={anonymous}
            onChange={(e) => setAnonymous(e.target.checked)}
          />

          <button
            type="submit"
            className="upload-card__submit"
            disabled={isSubmitting}
          >
            {isSubmitting ? "Uploading..." : "Upload"}
          </button>
        </form>
      </div>
    </div>
  );
}
