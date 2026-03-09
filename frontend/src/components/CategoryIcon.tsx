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
 */
export function CategoryIcon({
  name,
  imageUrl,
  size = "sm",
  className = "",
}: CategoryIconProps) {
  const sizeClass = `category-icon--${size}`;

  if (imageUrl) {
    return (
      <img
        src={imageUrl}
        alt={name}
        className={`category-icon category-icon--img ${sizeClass} ${className}`.trim()}
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
