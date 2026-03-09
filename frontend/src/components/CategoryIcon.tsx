import { useState } from "react";
import "./category-icon.css";

interface CategoryIconProps {
  name: string;
  imageUrl?: string | null;
  size?: "sm" | "md" | "lg";
  className?: string;
}

/**
 * Displays a category image or a styled placeholder with the first letter of the category name.
 * Reusable across browse, detail, home, and admin pages.
 * Falls back to the letter placeholder if the image fails to load.
 */
export function CategoryIcon({
  name,
  imageUrl,
  size = "sm",
  className = "",
}: CategoryIconProps) {
  const [imgError, setImgError] = useState(false);
  const sizeClass = `category-icon--${size}`;

  if (imageUrl && !imgError) {
    return (
      <img
        src={imageUrl}
        alt={name}
        loading="lazy"
        className={`category-icon category-icon--img ${sizeClass} ${className}`.trim()}
        onError={() => setImgError(true)}
      />
    );
  }

  const letter = name.charAt(0).toUpperCase();

  return (
    <span
      className={`category-icon category-icon--placeholder ${sizeClass} ${className}`.trim()}
      title={name}
      aria-label={name}
    >
      {letter}
    </span>
  );
}
