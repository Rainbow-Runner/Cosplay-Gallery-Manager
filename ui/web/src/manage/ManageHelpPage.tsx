import { useIntl } from "react-intl";
import { Link } from "react-router-dom";

export function ManageHelpPage() {
  const intl = useIntl();
  const f = (id: string) => intl.formatMessage({ id });

  return <main className="manage-page manage-help-page">
    <header className="manage-heading"><div><p>{f("manage.help.eyebrow")}</p><h2>{f("manage.help.title")}</h2></div></header>
    <p className="manage-help-intro">{f("manage.help.intro")}</p>
    <div className="manage-help-layout">
      <nav className="manage-help-topics" aria-label={f("manage.help.topics")}>
        <strong>{f("manage.help.topics")}</strong>
        <a href="#libraries" aria-current="page">{f("manage.help.libraries.title")}</a>
      </nav>
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
    </div>
  </main>;
}
