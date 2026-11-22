import { Modal } from "./ui";

/** Asked before a guest abandons an unfinished photo session (Home, Start over, Cancel). It says what
 *  will happen to the photos, and the safe answer ("Keep going") is the default. */
export function LeaveDialog({ state, taken, busy, error, confirmLabel = "Leave", onKeep, onLeave }:
  { state: string; taken: number; busy: boolean; error?: string; confirmLabel?: string; onKeep: () => void; onLeave: () => void }) {
  const shooting = state === "CAPTURING" || state === "SAVING";
  return (
    <Modal title="Leave this photo session?" onClose={busy ? () => {} : onKeep}>
      <div className="modal-body">
        {shooting
          ? <p>Hold on — your photo is being {state === "SAVING" ? "saved" : "taken"}. You can leave in a moment.</p>
          : state === "REVIEW"
            ? <p>You have {taken} photo{taken === 1 ? "" : "s"} waiting. If you leave now they will <strong>not</strong> be made into your strip. The host keeps the originals.</p>
            : <p>Your photos have not been taken yet. If you leave now, this session ends.</p>}
        {error && <div className="note err" role="alert"><div>{error}</div></div>}
      </div>
      <footer>
        <button className="primary" autoFocus onClick={onKeep} disabled={busy}>Keep going</button>
        <button className="danger" onClick={onLeave} disabled={busy || shooting}>{busy ? "One moment…" : confirmLabel}</button>
      </footer>
    </Modal>
  );
}
