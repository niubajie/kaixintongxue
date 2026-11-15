import type { CapacitorConfig } from "@capacitor/cli";

const config: CapacitorConfig = {
  appId: "com.kaixintongxue.app",
  appName: "开心同学",
  webDir: "dist",
  bundledWebRuntime: false,
  android: {
    allowMixedContent: false
  },
  server: {
    androidScheme: "https"
  }
};

export default config;
