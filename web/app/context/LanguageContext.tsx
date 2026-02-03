"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react";
import { type Locale, messages } from "../i18n";

const STORAGE_KEY = "locale";

function getInitialLocale(): Locale {
  if (typeof window === "undefined") return "en";
  const stored = localStorage.getItem(STORAGE_KEY) as Locale | null;
  if (stored === "zh" || stored === "en") return stored;
  const browser = navigator.language?.toLowerCase() ?? "";
  return browser.startsWith("zh") ? "zh" : "en";
}

interface LanguageContextValue {
  locale: Locale;
  setLocale: (locale: Locale) => void;
  t: (key: string, params?: Record<string, string | number>) => string;
}

const LanguageContext = createContext<LanguageContextValue | null>(null);

export function LanguageProvider({ children }: { children: React.ReactNode }) {
  const [locale, setLocaleState] = useState<Locale>("en");
  const [mounted, setMounted] = useState(false);

  useEffect(() => {
    queueMicrotask(() => {
      setLocaleState(getInitialLocale());
      setMounted(true);
    });
  }, []);

  useEffect(() => {
    if (!mounted || typeof document === "undefined") return;
    document.documentElement.lang = locale === "zh" ? "zh-CN" : "en";
  }, [mounted, locale]);

  const setLocale = useCallback((next: Locale) => {
    setLocaleState(next);
    if (typeof window !== "undefined") {
      localStorage.setItem(STORAGE_KEY, next);
    }
  }, []);

  const t = useCallback(
    (key: string, params?: Record<string, string | number>) => {
      let str = messages[locale][key] ?? key;
      if (params) {
        for (const [k, v] of Object.entries(params)) {
          str = str.replace(new RegExp(`\\{${k}\\}`, "g"), String(v));
        }
      }
      return str;
    },
    [locale]
  );

  const value = useMemo<LanguageContextValue>(
    () => ({ locale, setLocale, t }),
    [locale, setLocale, t]
  );

  return (
    <LanguageContext.Provider value={value}>{children}</LanguageContext.Provider>
  );
}

export function useLanguage() {
  const ctx = useContext(LanguageContext);
  if (!ctx) throw new Error("useLanguage must be used within LanguageProvider");
  return ctx;
}
