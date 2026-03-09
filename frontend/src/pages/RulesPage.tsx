import "./rules.css";

interface RulesSection {
  title: string;
  rules: string[];
}

const RULES_SECTIONS: RulesSection[] = [
  {
    title: "1. General Rules",
    rules: [
      "Treat all members with respect. Harassment, hate speech, and personal attacks will not be tolerated.",
      "Do not create multiple accounts. One account per person. Duplicate accounts will be banned.",
      "Do not share your account credentials or passkey with anyone.",
      "Do not attempt to exploit, hack, or disrupt the site or tracker in any way.",
      "Staff decisions are final. If you disagree, appeal through private message — do not argue publicly.",
      "Do not impersonate staff members or other users.",
      "English is the primary language of the site. Use English in all public areas.",
    ],
  },
  {
    title: "2. Uploading Rules",
    rules: [
      "Only upload content that is allowed in the site categories. No prohibited content.",
      "Do not upload duplicates. Search the site before uploading to ensure the content does not already exist.",
      "Provide accurate and complete information: title, description, category, and NFO when applicable.",
      "Torrents must be well-seeded after upload. You must seed your uploads for at least 72 hours or until a reasonable number of peers have completed the download.",
      "Do not upload corrupt, incomplete, or password-protected archives without clearly stating so in the description.",
      "Use proper naming conventions. No excessive caps, special characters, or misleading titles.",
      "RAR-packed releases are discouraged unless standard for the content type.",
    ],
  },
  {
    title: "3. Downloading Rules",
    rules: [
      "Seed back what you download. Maintain a healthy ratio at all times.",
      "Do not manipulate your upload or download statistics. Cheating is permanently bannable.",
      "Do not use your passkey in public or share .torrent files downloaded from this site.",
      "Use only approved BitTorrent clients. Modified or spoofing clients are banned.",
      "Do not redistribute content downloaded from this tracker on public trackers or other sites.",
    ],
  },
  {
    title: "4. Chat Rules",
    rules: [
      "Keep chat civil and on-topic. Excessive spam will result in a mute.",
      "No advertising, soliciting, or posting referral links.",
      "Do not request uploads, invites, or personal information in chat.",
      "Spoilers must be clearly marked. Be considerate of others.",
      "Do not flood the chat with repeated messages or excessive use of formatting.",
    ],
  },
  {
    title: "5. Ratio Requirements",
    rules: [
      "Your ratio (uploaded / downloaded) must stay above the minimum threshold. The current minimum ratio is 0.3." /* TODO: fetch from site settings */,
      "New accounts receive a grace period before ratio enforcement begins. Use this time to build your ratio.",
      "If your ratio drops below the minimum, your download privileges will be restricted until it recovers.",
      "Free Leech torrents do not count against your download total and are a great way to build ratio.",
      "If you are struggling with ratio, seed actively and consider uploading new content to the site.",
      "Ratio cheating (faking upload statistics) results in an immediate permanent ban with no appeal.",
    ],
  },
];

export function RulesPage() {
  return (
    <div className="rules">
      <h1 className="rules__title">Site Rules</h1>
      <p className="rules__subtitle">
        All members are expected to follow these rules. Ignorance is not an
        excuse.
      </p>

      {RULES_SECTIONS.map((section) => (
        <section key={section.title} className="rules__section">
          <h2 className="rules__section-title">{section.title}</h2>
          <ol className="rules__list">
            {section.rules.map((rule) => (
              <li key={rule} className="rules__item">
                {rule}
              </li>
            ))}
          </ol>
        </section>
      ))}

      <div className="rules__warning">
        <div className="rules__warning-title">Consequences for Violations</div>
        <p className="rules__warning-intro">
          Rule violations are handled on a case-by-case basis. Typical
          consequences include:
        </p>
        <p className="rules__warning-item">
          <strong>First offense:</strong> Warning issued to your account.
        </p>
        <p className="rules__warning-item">
          <strong>Second offense:</strong> Temporary restriction of privileges
          (download, upload, or chat).
        </p>
        <p className="rules__warning-item">
          <strong>Third offense:</strong> Extended ban or permanent account
          termination.
        </p>
        <p className="rules__warning-note">
          Severe violations (cheating, exploits, illegal content) may result in
          an immediate permanent ban without prior warnings.
        </p>
      </div>
    </div>
  );
}
