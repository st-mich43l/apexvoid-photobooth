import { useEffect, useState } from "react";
import { ShootFlow } from "../ShootFlow";
import { usePrepareMode } from "../usePrepareMode";

export function PhotoPage() {
  const { prepare, dialog, error } = usePrepareMode();
  const [ready, setReady] = useState(false);
  useEffect(() => { prepare("photo").then(setReady); }, []); // eslint-disable-line react-hooks/exhaustive-deps
  return (
    <div className="page">
      <h1>Photo session</h1>
      {error && <div className="note err" role="alert"><div>{error}</div></div>}
      {ready ? <ShootFlow variant="studio" /> : !error && <p className="muted">Preparing the photo camera…</p>}
      {dialog}
    </div>
  );
}
