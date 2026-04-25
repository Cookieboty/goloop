import type { Metadata } from "next";
import { AuthGuard } from "@/components/AuthGuard";
import { AppShell } from "@/components/AppShell";
import { DialogProvider } from "@/components/Dialog";
import "./globals.css";

export const metadata: Metadata = {
  title: "goloop Admin Console",
  description: "Multi-channel AI Gateway Admin UI",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="zh-CN">
      <body>
        <DialogProvider>
          <AuthGuard>
            <AppShell>{children}</AppShell>
          </AuthGuard>
        </DialogProvider>
      </body>
    </html>
  );
}
