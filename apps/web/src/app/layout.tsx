import type { Metadata } from "next";
import { Inter } from "next/font/google";

import { ThemeScript } from "@/components/theme/theme-script";
import { ToastProvider } from "@/components/ui";

import "./globals.css";

/** DESIGN.md: he thong dung DUY NHAT Inter. `variable` de tokens.css tham chieu qua
 *  --font-inter trong --font-sans. */
const inter = Inter({
  subsets: ["latin", "vietnamese"],
  variable: "--font-inter",
  display: "swap",
});

export const metadata: Metadata = {
  title: "EventFlow",
  description: "Nen tang ban ve su kien chiu tai dot bien",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="vi" className={inter.variable} suppressHydrationWarning>
      <head>
        {/* Phai nam trong <head> va chay sync: dat data-theme truoc paint dau tien,
          * neu khong trang se nhay tu light sang dark (FOUC). */}
        <ThemeScript />
      </head>
      <body>
        <ToastProvider>{children}</ToastProvider>
      </body>
    </html>
  );
}
