import type { Instance } from "../types";
import { factsEntries } from "../format";

export function FactsPanel({ instance }: { instance: Instance }) {
  return (
    <section className="card">
      <h2>状态与道具</h2>
      <dl className="facts">
        {factsEntries(instance).map(([key, value]) => (
          <div className="fact" key={key}>
            <dt>{key}</dt>
            <dd>{value}</dd>
          </div>
        ))}
      </dl>
      <div className="version">
        剧情版本 <code>{instance.version}</code>
        <br />
        修订 <code>{instance.rev}</code> · 访问 <code>{instance.visitId}</code>
      </div>
    </section>
  );
}
