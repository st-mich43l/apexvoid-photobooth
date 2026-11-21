import { useState } from "react";
import { Booth } from "./Booth";
import { Studio } from "./Studio";

export default function App() {
  const [mode, setMode] = useState<"studio" | "booth">(() => (location.hash.startsWith("#booth") ? "booth" : "studio"));
  return mode === "booth" ? <Booth onExit={() => setMode("studio")} /> : <Studio onBooth={() => setMode("booth")} />;
}
