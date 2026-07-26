import { useIntl } from "react-intl";

export function ProductShell({ kind }: { kind: "manage" | "setup" }) {
  const intl = useIntl();
  return <main className={`app-shell app-shell--${kind}`} data-testid={`${kind}-shell`}><div className="app-shell__content"><p className="app-shell__eyebrow">Cosplay Gallery Manager</p><h1>{intl.formatMessage({ id: `shell.${kind}.title` })}</h1><p>{intl.formatMessage({ id: `shell.${kind}.description` })}</p></div></main>;
}
