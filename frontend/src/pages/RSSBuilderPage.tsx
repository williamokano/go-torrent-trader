import { useEffect, useState } from "react";
import { api } from "@/api";
import { Select } from "@/components/form";
import { useAuth } from "@/features/auth";
import { getConfig } from "@/config";
import "./rss-builder.css";

function buildFeedURL(passkey: string, categoryId: string): string {
  const config = getConfig();
  let url = `${config.API_URL}/api/v1/rss?passkey=${passkey}`;
  if (categoryId) {
    url += `&cat=${categoryId}`;
  }
  return url;
}

export function RSSBuilderPage() {
  const { user } = useAuth();
  const [categoryId, setCategoryId] = useState("");
  const [copied, setCopied] = useState(false);
  const [categoryOptions, setCategoryOptions] = useState<
    { value: string; label: string }[]
  >([{ value: "", label: "All categories" }]);

  useEffect(() => {
    async function fetchCategories() {
      const { data } = await api.GET("/api/v1/categories");
      if (data?.categories) {
        const opts = [
          { value: "", label: "All categories" },
          ...data.categories.map((c) => ({
            value: String(c.id ?? ""),
            label: c.name ?? "Unknown",
          })),
        ];
        setCategoryOptions(opts);
      }
    }
    fetchCategories();
  }, []);

  const feedURL = user?.passkey ? buildFeedURL(user.passkey, categoryId) : "";

  async function handleCopy() {
    if (!feedURL) return;
    try {
      await navigator.clipboard.writeText(feedURL);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // Fallback for older browsers
      const textarea = document.createElement("textarea");
      textarea.value = feedURL;
      document.body.appendChild(textarea);
      textarea.select();
      document.execCommand("copy");
      document.body.removeChild(textarea);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }
  }

  if (!user?.passkey) {
    return (
      <div className="rss-builder">
        <div className="rss-builder__card">
          <h1 className="rss-builder__title">RSS Feed</h1>
          <p className="rss-builder__no-passkey">
            You need a passkey to use RSS feeds. Please generate one in your
            settings.
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="rss-builder">
      <div className="rss-builder__card">
        <h1 className="rss-builder__title">RSS Feed</h1>
        <p className="rss-builder__description">
          Use this URL in your torrent client or RSS reader to automatically
          download new torrents.
        </p>

        <div className="rss-builder__form">
          <Select
            label="Category"
            value={categoryId}
            onChange={(e) => setCategoryId(e.target.value)}
            options={categoryOptions}
          />

          <div className="rss-builder__url-group">
            <span className="rss-builder__url-label">Your RSS Feed URL</span>
            <div className="rss-builder__url-wrapper">
              <input
                className="rss-builder__url-input"
                type="text"
                value={feedURL}
                readOnly
                aria-label="RSS feed URL"
              />
              <button
                className={`rss-builder__copy-btn${copied ? " rss-builder__copy-btn--copied" : ""}`}
                onClick={handleCopy}
                type="button"
              >
                {copied ? "Copied!" : "Copy URL"}
              </button>
            </div>
          </div>

          <p className="rss-builder__warning">
            This URL contains your personal passkey. Do not share it with
            others. If your passkey is compromised, regenerate it in your
            account settings.
          </p>
        </div>
      </div>
    </div>
  );
}
