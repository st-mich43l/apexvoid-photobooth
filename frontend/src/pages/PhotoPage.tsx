import { ShootFlow } from "../ShootFlow";

export function PhotoPage() {
  return (
    <div className="page">
      <h1>Photo session</h1>
      <ShootFlow variant="studio" />
    </div>
  );
}
