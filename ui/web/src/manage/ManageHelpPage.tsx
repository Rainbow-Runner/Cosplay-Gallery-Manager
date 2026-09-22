import { useIntl } from "react-intl";
import { Link } from "react-router-dom";

const sourceAvailability = ["AVAILABLE", "MISSING", "UNREADABLE"] as const;
const sourceReconciliation = ["NEVER_SCANNED", "SCANNING", "IN_SYNC", "NEEDS_RESCAN", "ERROR"] as const;
const manifestStatuses = ["UNCHECKED", "STALE", "NONE", "CLEAN", "DB_DIRTY", "FILE_DIRTY", "CONFLICT", "MISSING", "ERROR", "SOURCE_UNAVAILABLE"] as const;
const recognitionSteps = ["bound", "manifest", "archive", "marker", "template", "depth", "unassigned"] as const;

export function ManageHelpPage() {
  const intl = useIntl();
  const f = (id: string) => intl.formatMessage({ id });

  return <main className="manage-page manage-help-page">
    <header className="manage-heading"><div><p>{f("manage.help.eyebrow")}</p><h2>{f("manage.help.title")}</h2></div></header>
    <p className="manage-help-intro">{f("manage.help.intro")}</p>
    <div className="manage-help-layout">
      <nav className="manage-help-topics" aria-label={f("manage.help.topics")}>
        <strong>{f("manage.help.topics")}</strong>
        <a href="#docker-paths">{f("manage.help.docker.title")}</a>
        <a href="#libraries">{f("manage.help.libraries.title")}</a>
        <a href="#scan-rules">{f("manage.help.rules.title")}</a>
        <a href="#gallery-status">{f("manage.help.gallery.title")}</a>
      </nav>
      <article id="docker-paths" className="manage-help-article"><header><h3>{f("manage.help.docker.title")}</h3><span>{f("manage.help.docker.summary")}</span></header><section><ul className="manage-help-list"><li>{f("manage.help.docker.state")}</li><li>{f("manage.help.docker.cache")}</li><li>{f("manage.help.docker.media")}</li><li>{f("manage.help.docker.transfer")}</li><li>{f("manage.help.docker.recovery")}</li></ul></section></article>
      <article id="libraries" className="manage-help-article">
        <header>
          <p>{f("manage.help.libraries.eyebrow")}</p>
          <h3>{f("manage.help.libraries.title")}</h3>
          <span>{f("manage.help.libraries.summary")}</span>
          <div className="manage-help-actions"><Link to="/manage/libraries">{f("manage.help.openLibraries")}</Link><Link to="/manage/settings">{f("manage.help.openSchedule")}</Link></div>
        </header>

        <section aria-labelledby="help-actions-title">
          <h4 id="help-actions-title">{f("manage.help.actions.title")}</h4>
          <div className="manage-help-card-grid">
            <article><strong>{f("manage.help.actions.review.title")}</strong><p>{f("manage.help.actions.review.body")}</p><small>{f("manage.help.actions.review.boundary")}</small></article>
            <article><strong>{f("manage.help.actions.automation.title")}</strong><p>{f("manage.help.actions.automation.body")}</p><small>{f("manage.help.actions.automation.boundary")}</small></article>
            <article><strong>{f("manage.help.actions.schedule.title")}</strong><p>{f("manage.help.actions.schedule.body")}</p><small>{f("manage.help.actions.schedule.boundary")}</small></article>
          </div>
        </section>

        <section aria-labelledby="help-scans-title">
          <h4 id="help-scans-title">{f("manage.help.scans.title")}</h4>
          <p>{f("manage.help.scans.intro")}</p>
          <div className="manage-help-table-wrap"><table>
            <thead><tr><th>{f("manage.help.table.layer")}</th><th>{f("manage.help.table.scope")}</th><th>{f("manage.help.table.effect")}</th><th>{f("manage.help.table.excludes")}</th></tr></thead>
            <tbody>
              <tr><th>{f("manage.help.scans.discovery.name")}</th><td>{f("manage.help.scans.discovery.scope")}</td><td>{f("manage.help.scans.discovery.effect")}</td><td>{f("manage.help.scans.discovery.excludes")}</td></tr>
              <tr><th>{f("manage.help.scans.source.name")}</th><td>{f("manage.help.scans.source.scope")}</td><td>{f("manage.help.scans.source.effect")}</td><td>{f("manage.help.scans.source.excludes")}</td></tr>
            </tbody>
          </table></div>
        </section>

        <section aria-labelledby="help-states-title">
          <h4 id="help-states-title">{f("manage.help.states.title")}</h4>
          <div className="manage-help-table-wrap"><table>
            <thead><tr><th>{f("manage.help.table.state")}</th><th>{f("manage.help.table.explicit")}</th><th>{f("manage.help.table.scheduled")}</th></tr></thead>
            <tbody>
              <tr><th>{f("manage.help.states.new.name")}</th><td>{f("manage.help.states.new.explicit")}</td><td>{f("manage.help.states.new.scheduled")}</td></tr>
              <tr><th>{f("manage.help.states.draft.name")}</th><td>{f("manage.help.states.draft.explicit")}</td><td>{f("manage.help.states.draft.scheduled")}</td></tr>
              <tr><th>{f("manage.help.states.active.name")}</th><td>{f("manage.help.states.active.explicit")}</td><td>{f("manage.help.states.active.scheduled")}</td></tr>
            </tbody>
          </table></div>
        </section>

        <section className="manage-help-safety" aria-labelledby="help-safety-title">
          <h4 id="help-safety-title">{f("manage.help.safety.title")}</h4>
          <ul><li>{f("manage.help.safety.explicit")}</li><li>{f("manage.help.safety.snapshot")}</li><li>{f("manage.help.safety.modes")}</li><li>{f("manage.help.safety.activation")}</li><li>{f("manage.help.safety.review")}</li></ul>
        </section>
      </article>
      <article id="scan-rules" className="manage-help-article">
        <header>
          <p>{f("manage.help.rules.eyebrow")}</p>
          <h3>{f("manage.help.rules.title")}</h3>
          <span>{f("manage.help.rules.summary")}</span>
          <div className="manage-help-actions"><Link to="/manage/libraries">{f("manage.help.openLibraries")}</Link></div>
        </header>
        <section aria-labelledby="help-rules-discovery-title">
          <h4 id="help-rules-discovery-title">{f("manage.help.rules.discovery.title")}</h4>
          <p>{f("manage.help.rules.discovery.intro")}</p>
          <div className="manage-help-table-wrap"><table><thead><tr><th>{f("manage.help.rules.order")}</th><th>{f("manage.help.gallery.meaning")}</th></tr></thead><tbody>
            {recognitionSteps.map((step) => <tr key={step}><th>{f(`manage.help.rules.discovery.${step}.name`)}</th><td>{f(`manage.help.rules.discovery.${step}.body`)}</td></tr>)}
          </tbody></table></div>
          <p>{f("manage.help.rules.discovery.draft")}</p>
        </section>
        <section aria-labelledby="help-rules-exclusions-title">
          <h4 id="help-rules-exclusions-title">{f("manage.help.rules.exclusion.title")}</h4>
          <p>{f("manage.help.rules.exclusion.intro")}</p>
          <ul className="manage-help-list">
            <li>{f("manage.help.rules.exclusion.root")}</li>
            <li>{f("manage.help.rules.exclusion.custom")}</li>
            <li>{f("manage.help.rules.exclusion.archive")}</li>
            <li>{f("manage.help.rules.exclusion.noBlacklist")}</li>
            <li>{f("manage.help.rules.exclusion.skipped")}</li>
          </ul>
        </section>
      </article>
      <article id="gallery-status" className="manage-help-article">
        <header>
          <p>{f("manage.help.gallery.eyebrow")}</p>
          <h3>{f("manage.help.gallery.title")}</h3>
          <span>{f("manage.help.gallery.summary")}</span>
          <div className="manage-help-actions"><Link to="/manage/gallery">{f("manage.help.gallery.open")}</Link></div>
        </header>
        <section aria-labelledby="help-gallery-source-title">
          <h4 id="help-gallery-source-title">{f("manage.help.gallery.source.title")}</h4>
          <p>{f("manage.help.gallery.source.intro")}</p>
          <div className="manage-help-table-wrap"><table><thead><tr><th>{f("manage.help.gallery.code")}</th><th>{f("manage.help.gallery.meaning")}</th></tr></thead><tbody>
            {sourceAvailability.map((code) => <tr key={code}><th>{code}</th><td>{f(`manage.help.gallery.source.availability.${code}`)}</td></tr>)}
          </tbody></table></div>
          <p>{f("manage.help.gallery.source.reconcileIntro")}</p>
          <div className="manage-help-table-wrap"><table><thead><tr><th>{f("manage.help.gallery.code")}</th><th>{f("manage.help.gallery.meaning")}</th></tr></thead><tbody>
            {sourceReconciliation.map((code) => <tr key={code}><th>{code}</th><td>{f(`manage.help.gallery.source.reconcile.${code}`)}</td></tr>)}
          </tbody></table></div>
          <p>{f("manage.help.gallery.source.errorCode")}</p>
        </section>
        <section aria-labelledby="help-gallery-manifest-title">
          <h4 id="help-gallery-manifest-title">{f("manage.help.gallery.manifest.title")}</h4>
          <p>{f("manage.help.gallery.manifest.intro")}</p>
          <div className="manage-help-table-wrap"><table><thead><tr><th>{f("manage.help.gallery.code")}</th><th>{f("manage.help.gallery.meaning")}</th></tr></thead><tbody>
            {manifestStatuses.map((code) => <tr key={code}><th>{code}</th><td>{f(`manage.help.gallery.manifest.${code}`)}</td></tr>)}
          </tbody></table></div>
          <p>{f("manage.help.gallery.manifest.checkedAt")}</p>
          <p>{f("manage.help.gallery.manifest.actions")}</p>
        </section>
      </article>
    </div>
  </main>;
}
