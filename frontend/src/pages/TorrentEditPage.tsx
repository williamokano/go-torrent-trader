import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { api } from "@/api";
import { Input, Select, Textarea, Checkbox } from "@/components/form";
import { useToast } from "@/components/toast";
import { useAuth } from "@/features/auth";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";
import type { Torrent } from "@/types/torrent";
import { buildCategoryOptions } from "@/utils/categories";
import "./torrent-edit.css";

export function TorrentEditPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const toast = useToast();
  const { user } = useAuth();

  const [torrent, setTorrent] = useState<Torrent | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [categoryOptions, setCategoryOptions] = useState<
    { value: string; label: string }[]
  >([{ value: "", label: "Select a category" }]);

  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [categoryId, setCategoryId] = useState("");
  const [anonymous, setAnonymous] = useState(false);
  const [nfo, setNfo] = useState("");
  const [banned, setBanned] = useState(false);
  const [free, setFree] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);

  useEffect(() => {
    async function fetchCategories() {
      const { data } = await api.GET("/api/v1/categories");
      if (data?.categories) {
        setCategoryOptions(
          buildCategoryOptions(
            data.categories as {
              id: number;
              name: string;
              parent_id: number | null;
              sort_order: number;
            }[],
            "Select a category",
          ),
        );
      }
    }
    fetchCategories();
  }, []);

  useEffect(() => {
    let cancelled = false;

    async function fetchTorrent() {
      setLoading(true);
      setError(null);

      const torrentId = Number(id);
      if (!id || isNaN(torrentId)) {
        setError("Invalid torrent ID");
        setLoading(false);
        return;
      }

      const token = getAccessToken();
      const { data, error: apiError } = await api.GET("/api/v1/torrents/{id}", {
        params: { path: { id: torrentId } },
        headers: token ? { Authorization: `Bearer ${token}` } : {},
      });

      if (cancelled) return;

      if (apiError) {
        const msg =
          (apiError as { error?: { message?: string } }).error?.message ??
          "Failed to load torrent";
        setError(msg);
        setLoading(false);
        return;
      }

      const t = data?.torrent ?? null;
      if (!t) {
        setError("Torrent not found");
        setLoading(false);
        return;
      }

      // Redirect if not owner and not admin
      if (user && t.uploader_id !== user.id && !user.isAdmin) {
        navigate(`/torrent/${id}`, { replace: true });
        return;
      }

      setTorrent(t);
      setName(t.name ?? "");
      setDescription(t.description ?? "");
      setNfo(((t as Record<string, unknown>).nfo as string) ?? "");
      setCategoryId(String(t.category_id ?? ""));
      setAnonymous(t.anonymous ?? false);
      setLoading(false);
    }

    fetchTorrent();
    return () => {
      cancelled = true;
    };
  }, [id]);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();

    if (!name.trim()) {
      toast.error("Name is required");
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
        toast.error("You must be logged in to edit");
        return;
      }

      const body: Record<string, unknown> = {
        name: name.trim(),
        description: description.trim(),
        nfo: nfo.trim(),
        category_id: Number(categoryId),
        anonymous,
      };

      if (user?.isAdmin) {
        body.banned = banned;
        body.free = free;
      }

      const response = await fetch(
        `${getConfig().API_URL}/api/v1/torrents/${encodeURIComponent(id!)}`,
        {
          method: "PUT",
          headers: {
            "Content-Type": "application/json",
            Authorization: `Bearer ${token}`,
          },
          body: JSON.stringify(body),
        },
      );

      if (!response.ok) {
        const data = await response.json().catch(() => null);
        const message =
          data?.error?.message ?? `Update failed (${response.status})`;
        throw new Error(message);
      }

      toast.success("Torrent updated successfully!");
      navigate(`/torrent/${id}`);
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : "Update failed. Please try again.",
      );
    } finally {
      setIsSubmitting(false);
    }
  }

  if (loading) {
    return (
      <div className="torrent-edit-page">
        <div className="torrent-edit-card__loading">Loading torrent...</div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="torrent-edit-page">
        <div className="torrent-edit-card__error">{error}</div>
      </div>
    );
  }

  if (!torrent) {
    return (
      <div className="torrent-edit-page">
        <div className="torrent-edit-card__error">Torrent not found</div>
      </div>
    );
  }

  return (
    <div className="torrent-edit-page">
      <div className="torrent-edit-card">
        <h1 className="torrent-edit-card__title">Edit Torrent</h1>
        <form className="torrent-edit-card__form" onSubmit={handleSubmit}>
          <Input
            label="Name"
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
          />

          <Textarea
            label="Description"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            rows={5}
          />

          <Textarea
            label="NFO"
            value={nfo}
            onChange={(e) => setNfo(e.target.value)}
            rows={6}
            placeholder="Paste NFO content (optional)"
          />

          <Select
            label="Category"
            options={categoryOptions}
            value={categoryId}
            onChange={(e) => setCategoryId(e.target.value)}
          />

          <Checkbox
            label="Upload anonymously"
            checked={anonymous}
            onChange={(e) => setAnonymous(e.target.checked)}
          />

          {user?.isAdmin && (
            <div className="torrent-edit-card__admin-section">
              <span className="torrent-edit-card__admin-title">
                Admin Controls
              </span>
              <Checkbox
                label="Banned"
                checked={banned}
                onChange={(e) => setBanned(e.target.checked)}
              />
              <Checkbox
                label="Freeleech"
                checked={free}
                onChange={(e) => setFree(e.target.checked)}
              />
            </div>
          )}

          <div className="torrent-edit-card__actions">
            <button
              type="button"
              className="torrent-edit-card__cancel"
              onClick={() => navigate(`/torrent/${id}`)}
            >
              Cancel
            </button>
            <button
              type="submit"
              className="torrent-edit-card__submit"
              disabled={isSubmitting}
            >
              {isSubmitting ? "Saving..." : "Save Changes"}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
