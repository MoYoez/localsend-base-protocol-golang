import type { Metadata } from "next";
import { Noto_Sans } from "next/font/google";
import { LanguageProvider } from "./context/LanguageContext";
import "./globals.css";

export const metadata: Metadata = {
  title: "Localsend-Go Downloader",
  description: "Localsend-Go Downloader",
  icons:{
    icon: [{url: "/favicon.ico", sizes: "any"}, {url: "/favicon.svg", sizes: "any"}],
  },
};

const NotoSans = Noto_Sans({
  weight: ["400", "700"],
  subsets: ["latin"],
  variable: "--font-noto-sans",
})

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en" className={NotoSans.variable} suppressHydrationWarning>
      <body
        className={`${NotoSans.variable} antialiased`}
      >
        <LanguageProvider>{children}</LanguageProvider>
      </body>
    </html>
  );
}
