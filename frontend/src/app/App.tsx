import { useState } from "react";
import { Booth } from "../features/booth/Booth";
import { Studio } from "../features/studio/Studio";

export default function App() {
  const [mode, setMode] = useState<"studio" | "booth">(() => (window.location.hash.startsWith("#booth") ? "booth" : "studio"));
  return mode === "booth" ? <Booth onExited={() => setMode("studio")} /> : <Studio onBooth={() => setMode("booth")} />;
}
