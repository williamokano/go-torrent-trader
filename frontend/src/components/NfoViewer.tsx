import "./nfo-viewer.css";

interface NfoViewerProps {
  content: string;
}

export function NfoViewer({ content }: NfoViewerProps) {
  return (
    <div className="nfo-viewer">
      <h2 className="nfo-viewer__title">NFO</h2>
      <pre className="nfo-viewer__content">{content}</pre>
    </div>
  );
}
