import { useState } from "react";
import "./faq.css";

interface FAQItem {
  question: string;
  answer: string;
}

interface FAQSection {
  title: string;
  items: FAQItem[];
}

const FAQ_SECTIONS: FAQSection[] = [
  {
    title: "Account",
    items: [
      {
        question: "How do I create an account?",
        answer:
          "Registration may be open or invite-only depending on site settings. If open, click Sign Up in the header. If invite-only, you will need an invitation from an existing member.",
      },
      {
        question: "How do I change my password?",
        answer:
          "Go to Settings from the header menu. Under the password section you can enter your current password and set a new one.",
      },
      {
        question: "I forgot my password. What do I do?",
        answer:
          'Click "Forgot Password" on the login page. Enter the email associated with your account and you will receive a password reset link.',
      },
      {
        question: "Can I change my username?",
        answer:
          "Usernames cannot be changed by users. Contact a site administrator if you need a username change.",
      },
    ],
  },
  {
    title: "Uploading",
    items: [
      {
        question: "How do I upload a torrent?",
        answer:
          "Navigate to Torrents > Upload. Fill in the required fields (name, category, description) and attach your .torrent file. Make sure the torrent is created with the tracker announce URL from your profile.",
      },
      {
        question: "What file types can I upload?",
        answer:
          "Only .torrent files are accepted. The content inside the torrent can be any type allowed by the site rules and categories.",
      },
      {
        question: "Can I edit my torrent after uploading?",
        answer:
          "Yes. Visit your torrent detail page and click Edit. You can update the name, description, category, and other metadata.",
      },
      {
        question: "How do I include an NFO file?",
        answer:
          "You can upload an NFO file alongside your torrent on the upload page. NFO files provide additional information about the release.",
      },
    ],
  },
  {
    title: "Downloading",
    items: [
      {
        question: "How do I download a torrent?",
        answer:
          "Browse the torrent listings and click the download button on the torrent detail page. The .torrent file will contain your personal passkey for tracking.",
      },
      {
        question: "Why is my download not starting?",
        answer:
          "Make sure there are seeders for the torrent. Check the peer list on the torrent detail page. Also verify your torrent client is configured correctly with no firewall blocks.",
      },
      {
        question: "Can I use any BitTorrent client?",
        answer:
          "Most standard BitTorrent clients work. Popular choices include qBittorrent, Deluge, and rTorrent. Check the rules page for any client restrictions.",
      },
    ],
  },
  {
    title: "Ratio",
    items: [
      {
        question: "What is ratio and why does it matter?",
        answer:
          "Ratio is the amount you have uploaded divided by the amount you have downloaded. Maintaining a healthy ratio ensures the community stays active. A ratio below the minimum threshold may result in download restrictions.",
      },
      {
        question: "How can I improve my ratio?",
        answer:
          "Seed your downloads for as long as possible. Upload new content. Download smaller torrents that have fewer seeders — you are more likely to upload data to other peers.",
      },
      {
        question: "What happens if my ratio gets too low?",
        answer:
          "You may lose the ability to download new torrents until your ratio recovers. Check the Ratio Requirements section on the Rules page for specific thresholds.",
      },
    ],
  },
  {
    title: "Rules & Moderation",
    items: [
      {
        question: "Where can I find the site rules?",
        answer:
          "Visit the Rules page from the Info menu in the header or from the footer links. The rules cover general conduct, uploading, downloading, and chat guidelines.",
      },
      {
        question: "What happens if I break a rule?",
        answer:
          "Depending on the severity, you may receive a warning, temporary restriction, or permanent ban. Repeated violations escalate consequences. Check the Rules page for details.",
      },
      {
        question: "How do I report a problem?",
        answer:
          "Use the Report button on any torrent or comment. For other issues, contact staff through private messages.",
      },
    ],
  },
  {
    title: "Technical",
    items: [
      {
        question: "What is a passkey?",
        answer:
          "A passkey is a unique identifier included in your torrent downloads. It allows the tracker to associate your activity with your account without requiring login in your torrent client.",
      },
      {
        question: "Can I use a seedbox?",
        answer:
          "Yes. Seedboxes are allowed. Make sure the torrent files you load on your seedbox contain your passkey.",
      },
      {
        question: "What is the announce URL?",
        answer:
          "The announce URL is the tracker address your torrent client communicates with. It is embedded in .torrent files downloaded from the site and includes your passkey.",
      },
      {
        question: "What ports should I open?",
        answer:
          "Your BitTorrent client listening port (typically in the 6881-6999 range) should be forwarded in your router for optimal connectivity. The exact port depends on your client configuration.",
      },
    ],
  },
];

function FAQItem({ item }: { item: FAQItem }) {
  const [open, setOpen] = useState(false);

  return (
    <div className="faq__item">
      <button
        className="faq__question"
        onClick={() => setOpen((prev) => !prev)}
        aria-expanded={open}
      >
        {item.question}
        <span className="faq__arrow">{open ? "\u25B4" : "\u25BE"}</span>
      </button>
      {open && <div className="faq__answer">{item.answer}</div>}
    </div>
  );
}

export function FAQPage() {
  return (
    <div className="faq">
      <h1 className="faq__title">Frequently Asked Questions</h1>
      <p className="faq__subtitle">
        Find answers to common questions about using the tracker.
      </p>

      {FAQ_SECTIONS.map((section) => (
        <section key={section.title} className="faq__section">
          <h2 className="faq__section-title">{section.title}</h2>
          {section.items.map((item) => (
            <FAQItem key={item.question} item={item} />
          ))}
        </section>
      ))}
    </div>
  );
}
