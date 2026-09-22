import { type FormEvent, useEffect, useMemo, useState } from "react";
import { useIntl } from "react-intl";
import { useNavigate } from "react-router-dom";

interface SetupInput {
  runtimeEnvironment: "NATIVE" | "DOCKER";
  locale: "zh-CN" | "en-GB";
  timezone: string;
  coserMetadataRoot: string;
  backupRoot: string;
  password: string;
}
interface SetupStatus { complete: boolean; runtimeEnvironment: "NATIVE" | "DOCKER"; ticketRequired: boolean; coserMetadataRoot: string; backupRoot: string }

export function SetupPage() {
  const intl = useIntl();
  const navigate = useNavigate();
  const detectedLocale = navigator.language.toLowerCase().startsWith("zh") ? "zh-CN" : "en-GB";
  const detectedTimezone = Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
  const [step, setStep] = useState(1);
  const [ticket, setTicket] = useState("");
  const [status, setStatus] = useState<SetupStatus | null>(null);
  const [passwordConfirmation, setPasswordConfirmation] = useState("");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [input, setInput] = useState<SetupInput>({ runtimeEnvironment: "NATIVE", locale: detectedLocale,
    timezone: detectedTimezone, coserMetadataRoot: "", backupRoot: "", password: "" });

  useEffect(() => {
    fetch("/setup/status", { credentials: "same-origin", cache: "no-store" }).then((response) => response.ok ? response.json() : Promise.reject())
      .then((value: SetupStatus | null) => { if (value?.complete) navigate("/login", { replace: true }); else if (value) {
        setStatus(value);
        setInput((current) => ({ ...current, runtimeEnvironment: value.runtimeEnvironment, coserMetadataRoot: value.coserMetadataRoot || current.coserMetadataRoot, backupRoot: value.backupRoot || current.backupRoot }));
      } })
      .catch(() => setError(intl.formatMessage({ id: "setup.statusUnavailable" })));
  }, [navigate, intl]);

  const canContinue = useMemo(() => {
    if (step === 1) return !!status && (!status.ticketRequired || ticket.trim().length > 20);
    if (step === 2) return input.password.length >= 8 && input.password === passwordConfirmation;
    if (step === 3) return input.timezone.length > 0;
    if (step === 4) return isAbsolutePath(input.coserMetadataRoot) && isAbsolutePath(input.backupRoot);
    return true;
  }, [input, passwordConfirmation, status, step, ticket]);

  async function next() {
    setError("");
    if (step === 1 && status?.ticketRequired) {
      const response = await fetch("/setup/ticket/exchange", { method: "POST", credentials: "same-origin",
        headers: { "Content-Type": "application/json" }, body: JSON.stringify({ ticket: ticket.trim() }) });
      if (!response.ok) { setError(intl.formatMessage({ id: "setup.ticketInvalid" })); return; }
      setTicket("");
    }
    setStep((current) => Math.min(5, current + 1));
  }

  async function complete(event: FormEvent) {
    event.preventDefault();
    setSubmitting(true); setError("");
    try {
      const response = await fetch("/setup/complete", { method: "POST", credentials: "same-origin",
        headers: { "Content-Type": "application/json" }, body: JSON.stringify(input) });
      if (!response.ok) {
        const detail = await response.text();
        if (detail.includes("Setup storage is unavailable")) throw new Error("storage");
        throw new Error("Setup failed");
      }
      window.location.assign("/login");
    } catch (failure) {
      setError(intl.formatMessage({ id: failure instanceof Error && failure.message === "storage" ? "setup.storageUnavailable" : "setup.failed" }));
    } finally { setSubmitting(false); }
  }

  return (
    <main className="setup-shell" data-testid="setup-shell">
      <form className="setup-card" onSubmit={complete}>
        <header><p>Cosplay Gallery Manager</p><h1>{intl.formatMessage({ id: "setup.title" })}</h1>
          <div className="setup-progress" role="progressbar" aria-label="Setup progress" aria-valuemin={1} aria-valuemax={5} aria-valuenow={step}>{[1, 2, 3, 4, 5].map((value) => <span key={value} className={value <= step ? "is-active" : ""} />)}</div>
        </header>
        {step === 1 ? <section><h2>{intl.formatMessage({ id: "setup.environment" })}</h2>
          <p>{input.runtimeEnvironment === "DOCKER" ? "Docker" : intl.formatMessage({ id: "setup.native" })}</p>
          {status?.ticketRequired ? <label className="field"><span>{intl.formatMessage({ id: "setup.ticket" })}</span><input type="password" value={ticket} onChange={(event) => setTicket(event.target.value)} autoComplete="one-time-code" /></label> : <p className="setup-note">{intl.formatMessage({ id: "setup.localNoTicket" })}</p>}
        </section> : null}
        {step === 2 ? <section><h2>{intl.formatMessage({ id: "setup.authentication" })}</h2>
          <label className="field"><span>{intl.formatMessage({ id: "setup.password" })}</span><input type="password" value={input.password} onChange={(event) => setInput({ ...input, password: event.target.value })} autoComplete="new-password" /></label>
          <label className="field"><span>{intl.formatMessage({ id: "setup.confirmPassword" })}</span><input type="password" value={passwordConfirmation} onChange={(event) => setPasswordConfirmation(event.target.value)} autoComplete="new-password" /></label>
        </section> : null}
        {step === 3 ? <section><h2>{intl.formatMessage({ id: "setup.interface" })}</h2>
          <label className="field"><span>{intl.formatMessage({ id: "setup.language" })}</span><select value={input.locale} onChange={(event) => setInput({ ...input, locale: event.target.value as SetupInput["locale"] })}><option value="zh-CN">中文</option><option value="en-GB">English</option></select></label>
          <label className="field"><span>{intl.formatMessage({ id: "setup.timezone" })}</span><input value={input.timezone} onChange={(event) => setInput({ ...input, timezone: event.target.value })} /></label>
        </section> : null}
        {step === 4 ? <section><h2>{intl.formatMessage({ id: "setup.storage" })}</h2>
          <label className="field"><span>{intl.formatMessage({ id: "setup.coserRoot" })}</span><input value={input.coserMetadataRoot} readOnly={input.runtimeEnvironment === "DOCKER"} onChange={(event) => setInput({ ...input, coserMetadataRoot: event.target.value })} placeholder="/data/cosers" /></label>
          <label className="field"><span>{intl.formatMessage({ id: "setup.backupRoot" })}</span><input value={input.backupRoot} readOnly={input.runtimeEnvironment === "DOCKER"} onChange={(event) => setInput({ ...input, backupRoot: event.target.value })} placeholder="/data/backups" /></label>
          {input.runtimeEnvironment === "DOCKER" ? <p className="setup-note">{intl.formatMessage({ id: "setup.dockerStorageNote" })}</p> : null}
        </section> : null}
        {step === 5 ? <section><h2>{intl.formatMessage({ id: "setup.confirm" })}</h2><dl className="setup-review">
          <dt>{intl.formatMessage({ id: "setup.environment" })}</dt><dd>{input.runtimeEnvironment}</dd>
          <dt>{intl.formatMessage({ id: "setup.language" })}</dt><dd>{input.locale}</dd>
          <dt>{intl.formatMessage({ id: "setup.timezone" })}</dt><dd>{input.timezone}</dd>
          <dt>{intl.formatMessage({ id: "setup.coserRoot" })}</dt><dd>{input.coserMetadataRoot}</dd>
          <dt>{intl.formatMessage({ id: "setup.backupRoot" })}</dt><dd>{input.backupRoot}</dd>
        </dl><p className="setup-note">{intl.formatMessage({ id: "setup.noAutomaticScan" })}</p></section> : null}
        {error ? <p className="form-error" role="alert">{error}</p> : null}
        <footer>{step > 1 ? <button type="button" onClick={() => setStep((current) => current - 1)}>{intl.formatMessage({ id: "setup.back" })}</button> : <span />}
          {step < 5 ? <button type="button" disabled={!canContinue} onClick={next}>{intl.formatMessage({ id: "setup.next" })}</button>
            : <button type="submit" disabled={submitting}>{intl.formatMessage({ id: "setup.create" })}</button>}</footer>
      </form>
    </main>
  );
}

function isAbsolutePath(value: string) {
  return value.startsWith("/") || /^[A-Za-z]:[\\/]/.test(value) || value.startsWith("\\\\");
}
