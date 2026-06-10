import { useState } from "react";

export function Preview({ url }: { url: string }) {
  const [failed, setFailed] = useState(false);

  return (
    <div className="preview">
      <div className="preview-bar">
        <a href={url} target="_blank" rel="noreferrer">
          {url} <span className="preview-open">↗ open</span>
        </a>
      </div>
      {failed ? (
        <div className="preview-error">
          preview unavailable — is the server up?
          <span className="preview-error-url">{url}</span>
        </div>
      ) : (
        <iframe
          className="preview-frame"
          src={url}
          title="preview"
          onError={() => setFailed(true)}
        />
      )}
    </div>
  );
}
