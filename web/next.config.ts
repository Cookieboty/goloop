import type { NextConfig } from "next";

// In development (npm run dev), we run without static export so that
// Next.js rewrites can proxy API calls to the Go backend on :8080.
// In production (npm run build), we export as pure static files for
// embedding into the Go binary via go:embed.
const isDev = process.env.NODE_ENV === "development";

const devConfig: NextConfig = {
  // No output: 'export' in dev — keeps rewrites working
  async rewrites() {
    return [
      {
        source: "/admin/:path*",
        destination: "http://localhost:8080/admin/:path*",
      },
      {
        source: "/v1beta/:path*",
        destination: "http://localhost:8080/v1beta/:path*",
      },
    ];
  },
};

const prodConfig: NextConfig = {
  output: "export",
  basePath: "/admin/ui",
  trailingSlash: true,
  images: { unoptimized: true },
};

const nextConfig: NextConfig = isDev ? devConfig : prodConfig;

export default nextConfig;
