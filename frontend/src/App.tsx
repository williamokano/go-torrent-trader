import { useTheme } from "@/themes";

function App() {
  const { theme, toggleTheme } = useTheme();

  return (
    <div style={{ padding: "var(--space-xl)" }}>
      <h1>TorrentTrader</h1>
      <p>Welcome to TorrentTrader 3.0</p>
      <button
        onClick={toggleTheme}
        style={{
          marginTop: "var(--space-md)",
          padding: "var(--space-sm) var(--space-md)",
          backgroundColor: "var(--color-accent)",
          color: "#fff",
          border: "none",
          borderRadius: "var(--radius-md)",
          cursor: "pointer",
          fontFamily: "var(--font-sans)",
          fontSize: "var(--text-base)",
        }}
      >
        Theme: {theme}
      </button>
    </div>
  );
}

export default App;
