import { useState } from "react";
import { FrameSelection } from "../FrameSelection";
import { ShootFlow } from "../ShootFlow";
import { usePrepareMode } from "../usePrepareMode";
import type { FrameChoice } from "../types";

/** Studio photo session: pick a frame first (it sets the number of photos), then prepare the camera and shoot. */
export function PhotoPage() {
  const { prepare, dialog, error } = usePrepareMode();
  const [frame, setFrame] = useState<FrameChoice | null>(null);
  const [shooting, setShooting] = useState(false);
  const [busy, setBusy] = useState(false);
  const confirm = async (c: FrameChoice) => {
    setBusy(true);
    try { if (await prepare("photo")) { setFrame(c); setShooting(true); } } finally { setBusy(false); }
  };
  return (
    <div className="page">
      {!shooting && <h1>Photo session</h1>}
      {error && <div className="note err" role="alert"><div>{error}</div></div>}
      {shooting && frame
        ? <ShootFlow variant="studio" frame={frame} onChangeFrame={() => setShooting(false)} onDone={() => { setShooting(false); }} />
        : <FrameSelection variant="studio" busy={busy} initial={frame ? { id: frame.id, variantId: frame.variantId, caption: frame.caption } : undefined}
            title="Choose a frame" confirmLabel="Prepare camera" onConfirm={confirm} />}
      {dialog}
    </div>
  );
}
