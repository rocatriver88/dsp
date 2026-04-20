import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  output: "standalone",
  images: {
    remotePatterns: [
      { protocol: "http", hostname: "localhost", port: "8181", pathname: "/**" },
      { protocol: "http", hostname: "localhost", port: "9181", pathname: "/**" },
      { protocol: "http", hostname: "localhost", port: "18181", pathname: "/**" },
      { protocol: "http", hostname: "127.0.0.1", port: "8181", pathname: "/**" },
      { protocol: "http", hostname: "127.0.0.1", port: "9181", pathname: "/**" },
      { protocol: "http", hostname: "127.0.0.1", port: "18181", pathname: "/**" },
    ],
  },
};

export default nextConfig;
