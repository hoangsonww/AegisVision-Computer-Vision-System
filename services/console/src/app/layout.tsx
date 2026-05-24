import type { Metadata } from 'next';
import './globals.css';
import { Providers } from './providers';
import { Sidebar } from '@/components/Sidebar';
import { TopBar } from '@/components/TopBar';
import { ToastContainer } from '@/components/Toast';

export const metadata: Metadata = {
  title: 'AegisVision Console',
  description: 'Operator console for AegisVision — every platform feature, one UI.',
  authors: [{ name: 'Son Nguyen', url: 'https://sonnguyenhoang.com' }],
  icons: { icon: '/favicon.svg' },
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <head>
        <link rel="preconnect" href="https://fonts.googleapis.com" />
        <link rel="preconnect" href="https://fonts.gstatic.com" crossOrigin="anonymous" />
        {/* eslint-disable-next-line @next/next/no-page-custom-font */}
        <link
          href="https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700;800&family=JetBrains+Mono:wght@400;500;600;700&family=Space+Grotesk:wght@400;500;600;700&display=swap"
          rel="stylesheet"
        />
      </head>
      <body className="stars">
        <Providers>
          <div className="flex min-h-screen">
            <Sidebar />
            <div className="flex-1 min-w-0">
              <TopBar />
              <main className="p-6 max-w-screen-2xl mx-auto">{children}</main>
            </div>
          </div>
          <ToastContainer />
        </Providers>
      </body>
    </html>
  );
}
