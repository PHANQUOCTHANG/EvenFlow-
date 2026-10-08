import path from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));

/** @type {import('next').NextConfig} */
const nextConfig = {
  // standalone: image nho, khoi dong nhanh. Quan trong vi pre-warm truoc gio mo
  // ban khong duoc phu thuoc vao pod khoi dong cham.
  output: "standalone",
  outputFileTracingRoot: path.join(__dirname, "../.."),
  reactStrictMode: true,
  poweredByHeader: false,
};

export default nextConfig;
