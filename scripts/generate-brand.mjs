// Generates the committed brand images in public/ from vector sources:
//   public/favicon.svg (hand-written), public/apple-touch-icon.png, public/favicon-32.png,
//   public/branding/logo.svg, public/branding/og-image.png (1200×630, with a real app screenshot).
// Run with: node scripts/generate-brand.mjs   (only needed when the brand changes)
import sharp from "sharp";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";

const mark = (size, bg = true) => `<svg xmlns="http://www.w3.org/2000/svg" width="${size}" height="${size}" viewBox="0 0 32 32">
  <defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#8B5CF6"/><stop offset="1" stop-color="#EC4899"/></linearGradient></defs>
  ${bg ? `<rect width="32" height="32" rx="9" fill="url(#g)"/>` : ""}
  <rect x="10.5" y="5" width="11" height="22" rx="2.2" fill="#fff"/>
  <rect x="12.6" y="7.2" width="6.8" height="5" rx="1" fill="#2a1b45"/>
  <rect x="12.6" y="13.5" width="6.8" height="5" rx="1" fill="#2a1b45"/>
  <rect x="12.6" y="19.8" width="6.8" height="5" rx="1" fill="#2a1b45"/>
</svg>`;

mkdirSync("public/branding", { recursive: true });
writeFileSync("public/favicon.svg", mark(32));
writeFileSync("public/branding/logo.svg", mark(256));
await sharp(Buffer.from(mark(180)))
  .resize(180, 180)
  .png()
  .toFile("public/apple-touch-icon.png");
await sharp(Buffer.from(mark(32)))
  .resize(32, 32)
  .png()
  .toFile("public/favicon-32.png");

// Open Graph card: brand on the left, a real (harness) screenshot of Booth Mode on the right.
const W = 1200,
  H = 630;
const shot = await sharp(readFileSync("src/assets/screenshots/booth-frames.en.png"))
  .resize(620, 400, { fit: "cover", position: "left top" })
  .png()
  .toBuffer();
const shotRounded = await sharp(shot)
  .composite([
    { input: Buffer.from(`<svg width="620" height="400"><rect width="620" height="400" rx="20" ry="20"/></svg>`), blend: "dest-in" },
  ])
  .png()
  .toBuffer();
const base = `<svg xmlns="http://www.w3.org/2000/svg" width="${W}" height="${H}">
  <defs>
    <radialGradient id="h" cx="0.75" cy="0.3" r="0.7"><stop offset="0" stop-color="#8B5CF6" stop-opacity="0.35"/><stop offset="1" stop-color="#09090F" stop-opacity="0"/></radialGradient>
  </defs>
  <rect width="${W}" height="${H}" fill="#09090F"/>
  <rect width="${W}" height="${H}" fill="url(#h)"/>
  <g transform="translate(72,84)">${mark(56)
    .replace(/<svg[^>]*>|<\/svg>/g, "")
    .replace("<defs>", "<defs>")}</g>
  <text x="72" y="236" font-family="Fraunces, 'Noto Serif', 'DejaVu Serif', Georgia, serif" font-size="50" font-weight="600" fill="#F5F5F7">Capture moments.</text>
  <text x="72" y="296" font-family="Fraunces, 'Noto Serif', 'DejaVu Serif', Georgia, serif" font-size="50" font-weight="600" fill="#C4B5FD">Create memories.</text>
  <text x="72" y="380" font-family="Inter, 'Noto Sans', 'DejaVu Sans', sans-serif" font-size="26" fill="#B4B4BD">Local-first photobooth software</text>
  <text x="72" y="414" font-family="Inter, 'Noto Sans', 'DejaVu Sans', sans-serif" font-size="26" fill="#B4B4BD">for Windows 10 and 11</text>
  <text x="72" y="548" font-family="Inter, 'Noto Sans', 'DejaVu Sans', sans-serif" font-size="24" font-weight="600" fill="#F5F5F7">ApexVoid Photobooth</text>
  <text x="72" y="580" font-family="Inter, 'Noto Sans', 'DejaVu Sans', sans-serif" font-size="20" fill="#9A9AA4">photobooth.apexvoid.net</text>
</svg>`;
// the logo mark is drawn at 56px: scale the 32-unit mark
const baseFixed = base.replace(`<g transform="translate(72,84)">`, `<g transform="translate(72,84) scale(1.75)">`);
await sharp(Buffer.from(baseFixed))
  .composite([{ input: shotRounded, left: 620, top: 115 }])
  .png({ compressionLevel: 9 })
  .toFile("public/branding/og-image.png");
console.log("brand images written");
