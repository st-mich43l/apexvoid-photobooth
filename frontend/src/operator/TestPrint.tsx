import { useState } from "react";
import { api } from "../api";
import { useApp } from "../state";
import { Confirm, ErrorNote, useAction } from "../ui";

/** A test print is always an explicit, confirmed operator action: it uses paper and ink. Never run at start-up or by a page opening. */
export function TestPrint({ printer, paper, disabled, onSent }: { printer: string; paper: string; disabled?: boolean; onSent?: () => void }) {
  const { toast } = useApp();
  const [ask, setAsk] = useState(false);
  const act = useAction();
  return (
    <>
      <button disabled={disabled || act.busy || !printer} onClick={() => setAsk(true)}>Test print…</button>
      <ErrorNote error={act.error} onClose={act.clear} />
      {ask && (
        <Confirm title="Print a test page?" confirmLabel="Print test page" onNo={() => setAsk(false)}
          body={<p>One calibration page will be sent to <strong>{printer}</strong> on <strong>{paper || "the default paper"}</strong>. It uses paper (and ink or ribbon). Windows will only tell us the job was accepted — check the output tray yourself.</p>}
          onYes={() => { setAsk(false); void act.run(async () => { const j = await api.testPrint(printer, paper, "portrait"); toast(`Test page sent to the printer (${j.id})`); onSent?.(); }); }} />
      )}
    </>
  );
}
