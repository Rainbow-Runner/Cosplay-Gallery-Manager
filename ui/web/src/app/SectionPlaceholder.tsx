import { useIntl } from "react-intl";

export function SectionPlaceholder({ titleID }: { titleID: string }) {
  const intl = useIntl();
  return <main className="browse-main"><header className="page-heading"><p>CGM</p><h1>{intl.formatMessage({ id: titleID })}</h1></header></main>;
}
