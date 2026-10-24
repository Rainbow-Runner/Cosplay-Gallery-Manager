import { Check, Copy } from "lucide-react";
import { useIntl } from "react-intl";
import type { MediaMetadataEvent } from "./types";

export function MediaHistoryTimeline({ events, chronological, copied, onCopy }: {
  events: MediaMetadataEvent[];
  chronological: boolean;
  copied: string;
  onCopy: (key: string, value: string) => void;
}) {
  const intl = useIntl();
  const t = (key: string) => intl.formatMessage({ id: `media.metadata.${key}` });
  return <>
    <p className="media-metadata-note">{t(chronological ? "timeOrder" : "sourceOrder")}</p>
    <ol className="media-metadata-history" data-order={chronological ? "time" : "source"} aria-label={t("timeline")}>
      {events.map(event => {
        const conversion = event.action === "derived" || event.action === "converted";
        // Read older cached responses without turning free text into formats.
        const legacyFormats = /^([a-z0-9.+-]+\/[a-z0-9.+-]+) → ([a-z0-9.+-]+\/[a-z0-9.+-]+)$/i.exec(event.parameters);
        const unknownFormat = event.rawParameters ? t("unparsed") : t("notRecorded");
        const fields = [
          ...(conversion ? [
            { key: "inputFormat", value: event.inputFormat || legacyFormats?.[1] || unknownFormat },
            { key: "outputFormat", value: event.outputFormat || legacyFormats?.[2] || unknownFormat },
          ] : []),
          { key: conversion ? "conversionTime" : "eventTime", value: event.when || t("unknownTime"), time: true },
          { key: conversion ? "conversionSoftware" : "eventSoftware", value: event.software || t("notRecorded") },
          ...(conversion || event.rawParameters ? [{ key: "originalDescription", value: event.rawParameters || t("notRecorded") }] : []),
          ...(!conversion || event.changed ? [{ key: "changedScope", value: event.changed || t("notRecorded") }] : []),
        ];
        const record = intl.formatMessage({ id: "media.metadata.eventRecord" }, { index: event.index });
        const copyKey = `history.${event.index}`;
        const copyValue = [record + " · XMP", `${t("operation")}: ${t("action." + event.action)} (${event.action})`,
          ...fields.map(field => `${t(field.key)}: ${field.value}`),
          ...(event.when && !event.timestamp ? [t("unzoned")] : []),
          ...(event.detailsRedacted ? [t("pathsHidden")] : []),
        ].join("\n");
        return <li key={event.index} aria-label={record}>
          <header className="media-metadata-event-heading">
            <span><small>{t("operation")}</small><strong title={event.action}>{t("action." + event.action)}</strong><small className="media-metadata-event-source">#{event.index} · XMP</small></span>
            <button className="media-metadata-event-copy" type="button" aria-label={`${t(copied === copyKey ? "copied" : "copy")} · ${record}`} title={t("copy")}
              onClick={() => onCopy(copyKey, copyValue)}>{copied === copyKey ? <Check size={14} aria-hidden="true"/> : <Copy size={14} aria-hidden="true"/>}</button>
          </header>
          <dl className="media-metadata-event-fields">{fields.map(field => <div key={field.key} data-event-property={field.key}>
            <dt>{t(field.key)}</dt><dd>{field.time && event.when ? <time dateTime={event.timestamp || undefined}>{field.value}</time> : field.value}
              {field.time && event.when && !event.timestamp ? <small className="media-metadata-indicator">{t("unzoned")}</small> : null}
            </dd>
          </div>)}</dl>
          {event.detailsRedacted ? <p className="media-metadata-event-note">{t("pathsHidden")}</p> : null}
        </li>;
      })}
    </ol>
  </>;
}
