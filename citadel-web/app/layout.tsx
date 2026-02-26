import type { Metadata } from "next";
import { JetBrains_Mono } from "next/font/google";
import "./globals.css";

const mono = JetBrains_Mono({
  weight: ["400", "500", "600", "700", "800"],
  subsets: ["latin"],
  variable: "--font-mono",
});

export const metadata: Metadata = {
  title: "Citadel – Local DNS Proxy for Screen Time & Content Control",
  description:
    "Citadel is a local DNS proxy appliance that lets parents control screen time, block unwanted content, and enforce per-device internet rules without any cloud dependency.",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en">
      <body
        className={`${mono.variable}`}
      >
        {children}
      </body>
    </html>
  );
}
