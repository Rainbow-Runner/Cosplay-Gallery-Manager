import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { useIntl } from "react-intl";

interface About {
  product: string;
  version: string;
  gitHash?: string;
  buildTime?: string;
  sourceCodeURL: string;
  exactSourceAvailable: boolean;
  license: string;
  warranty: string;
  attribution: string;
}

export function LegalPage() {
  const intl = useIntl();
  const [about, setAbout] = useState<About | null>(null);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    let active = true;
    fetch("/about.json", { cache: "no-store" })
      .then((response) => response.ok ? response.json() : Promise.reject())
      .then((value: About) => { if (active) setAbout(value); })
      .catch(() => { if (active) setFailed(true); });
    return () => { active = false; };
  }, []);

  return <main className="legal-page">
    <header><p>ABOUT / LEGAL</p><h1>Cosplay Gallery Manager</h1></header>
    {failed ? <p role="alert">{intl.formatMessage({ id: "legal.failed" })}</p> : null}
    {!about && !failed ? <p role="status">{intl.formatMessage({ id: "legal.loading" })}</p> : null}
    {about ? <>
      <dl>
        <dt>{intl.formatMessage({ id: "legal.version" })}</dt><dd>{about.version}</dd>
        <dt>{intl.formatMessage({ id: "legal.commit" })}</dt><dd><code>{about.gitHash || intl.formatMessage({ id: "legal.development" })}</code></dd>
        {about.buildTime ? <><dt>{intl.formatMessage({ id: "legal.buildTime" })}</dt><dd>{about.buildTime}</dd></> : null}
        <dt>{intl.formatMessage({ id: "legal.license" })}</dt><dd>{about.license}</dd>
      </dl>
      <section><h2>{intl.formatMessage({ id: "legal.source" })}</h2>
        <p>{about.exactSourceAvailable ? intl.formatMessage({ id: "legal.sourceExact" }) : intl.formatMessage({ id: "legal.sourceDevelopment" })}</p>
        <a href={about.sourceCodeURL} rel="noreferrer" target="_blank">{about.sourceCodeURL}</a>
      </section>
      <section><h2>{intl.formatMessage({ id: "legal.warranty" })}</h2><p>{about.warranty}</p></section>
      <section><h2>{intl.formatMessage({ id: "legal.attribution" })}</h2><p>{about.attribution}</p></section>
    </> : null}
    <footer><Link to="/">{intl.formatMessage({ id: "legal.return" })}</Link></footer>
  </main>;
}
