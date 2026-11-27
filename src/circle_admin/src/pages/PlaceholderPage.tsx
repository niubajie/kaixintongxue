interface Props {
  title: string;
  desc: string;
  planned: string[];
}

export default function PlaceholderPage({ title, desc, planned }: Props) {
  return (
    <div className="page">
      <section className="panel">
        <div className="panel-head">
          <h2>{title}</h2>
          <span className="badge badge-gray">接口规划中</span>
        </div>
        <div className="state-box state-empty">
          <p className="empty-title">后端接口尚未实现</p>
          <p className="empty-desc">{desc}</p>
          <div className="planned">
            <div className="planned-title">规划中的接口</div>
            <ul>
              {planned.map((p) => (
                <li key={p}>
                  <code>{p}</code>
                </li>
              ))}
            </ul>
          </div>
        </div>
      </section>
    </div>
  );
}
