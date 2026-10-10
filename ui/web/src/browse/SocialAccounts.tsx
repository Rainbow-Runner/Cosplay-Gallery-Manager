import type { SocialAccount } from "./types";
import { SocialPlatformIcon } from "./SocialPlatformIcon";

const referencePlatforms = [
  { key: "twitter", label: "X" },
  { key: "facebook", label: "Facebook" },
  { key: "instagram", label: "Instagram" },
  { key: "weibo", label: "微博" },
  { key: "patreon", label: "Patreon" },
  { key: "linktree", label: "Linktree" },
] as const;

function displayPlatformKey(account: SocialAccount) {
  const key = account.platformKey.toLowerCase();
  if (key === "x") return "twitter";
  if (key === "linktree") return "linktree";
  if (key === "website" && account.label.trim().toLowerCase() === "linktree") return "linktree";
  if (key === "website") {
    try {
      if (new URL(account.url).hostname.toLowerCase().replace(/^www\./, "") === "linktr.ee") return "linktree";
    } catch {
      // The backend already validates account URLs; retain the generic fallback
      // if an older imported row cannot be parsed in this browser.
    }
  }
  return key;
}

function AccountLink({ account, iconKey }: { account: SocialAccount; iconKey: string }) {
  const label = account.label || account.handle || account.platformKey;
  return <a
    className={`social-account${account.status === "INACTIVE" ? " is-inactive" : ""}`}
    href={account.url}
    target="_blank"
    rel="noreferrer"
    aria-label={`${account.platformKey}: ${label}`}
    title={label}
  ><SocialPlatformIcon platformKey={iconKey} /></a>;
}

export function SocialAccounts({ accounts }: { accounts: SocialAccount[] }) {
  const remaining = new Set(accounts);
  return <div className="social-accounts">
    {referencePlatforms.map((platform) => {
      const account = accounts.find((candidate) => remaining.has(candidate) && displayPlatformKey(candidate) === platform.key);
      if (account) {
        remaining.delete(account);
        return <AccountLink key={platform.key} account={account} iconKey={platform.key} />;
      }
      return <span key={platform.key} className="social-account is-missing" title={platform.label} aria-hidden="true">
        <SocialPlatformIcon platformKey={platform.key} />
      </span>;
    })}
    {accounts.filter((account) => remaining.has(account)).map((account) => (
      <AccountLink key={account.uuid} account={account} iconKey={displayPlatformKey(account)} />
    ))}
  </div>;
}
