/** @type {import('next').NextConfig} */
const nextConfig = {
  // standalone: image nho, khoi dong nhanh. Quan trong vi pre-warm truoc gio mo
  // ban khong duoc phu thuoc vao pod khoi dong cham.
  output: "standalone",
  reactStrictMode: true,
  poweredByHeader: false,
};

export default nextConfig;
