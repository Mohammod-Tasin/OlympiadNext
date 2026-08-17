"use client";

import Script from "next/script";
import { useRouter } from "next/navigation";
import { useCallback, useRef } from "react";
import { useAuth } from "@/lib/auth/useAuth";

declare global {
  interface Window {
    google?: {
      accounts: {
        id: {
          initialize: (config: {
            client_id: string;
            callback: (response: { credential: string }) => void;
          }) => void;
          renderButton: (parent: HTMLElement, options: { theme: string; size: string; width?: number }) => void;
        };
      };
    };
  }
}

export function GoogleSignInButton({ onError }: { onError?: (message: string) => void }) {
  const { loginWithGoogle } = useAuth();
  const router = useRouter();
  const containerRef = useRef<HTMLDivElement>(null);

  const handleCredential = useCallback(
    async (response: { credential: string }) => {
      try {
        await loginWithGoogle(response.credential);
        router.push("/dashboard");
      } catch {
        onError?.("Google sign-in failed. Please try again.");
      }
    },
    [loginWithGoogle, onError, router],
  );

  const initialize = useCallback(() => {
    const clientId = process.env.NEXT_PUBLIC_GOOGLE_CLIENT_ID;
    if (!clientId || !window.google || !containerRef.current) return;

    window.google.accounts.id.initialize({
      client_id: clientId,
      callback: handleCredential,
    });
    window.google.accounts.id.renderButton(containerRef.current, {
      theme: "outline",
      size: "large",
      width: 320,
    });
  }, [handleCredential]);

  return (
    <>
      <Script src="https://accounts.google.com/gsi/client" strategy="afterInteractive" onLoad={initialize} />
      <div ref={containerRef} />
    </>
  );
}
