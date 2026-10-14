"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useId, useRef, useState } from "react";

import { LanguageToggle } from "@/components/i18n/language-toggle";
import { useT } from "@/components/i18n/locale-provider";
import { ThemeToggle } from "@/components/theme/theme-toggle";
import { cn } from "@/lib/cn";

import { Container } from "./container";
import { PUBLIC_NAV, activeHref, type NavItem } from "./nav-config";
import { NavLink } from "./nav-link";

export function PublicHeader({ items = PUBLIC_NAV }: { items?: NavItem[] }) {
  const pathname = usePathname();
  const t = useT();
  const [open, setOpen] = useState(false);
  const toggleRef = useRef<HTMLButtonElement>(null);
  const navId = useId();

  const current = activeHref(items, pathname);

  // Dieu huong xong thi dong menu, neu khong no se che mat trang vua mo.
  useEffect(() => {
    setOpen(false);
  }, [pathname]);

  useEffect(() => {
    if (!open) return;
    function onKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        setOpen(false);
        // Tra focus ve nut da mo menu, neu khong nguoi dung ban phim bi mat vi tri.
        toggleRef.current?.focus();
      }
    }
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [open]);

  return (
    <header className="border-b border-border bg-surface">
      {/* flex-wrap + min-h: o 320px logo + nhom ngon ngu + theme + nut menu khong vua mot hang; cho xuong dong
          * thay vi tran ngang (WCAG 1.4.10 Reflow). */}
      <Container className="flex min-h-16 flex-wrap items-center justify-between gap-x-md gap-y-sm py-sm">
        <Link href="/" className="ef-focus-ring rounded-control text-headline-md text-fg">
          EventFlow
        </Link>

        <div className="flex flex-wrap items-center gap-sm">
          <LanguageToggle />
          <ThemeToggle />
          <button
            ref={toggleRef}
            type="button"
            aria-expanded={open}
            aria-controls={navId}
            onClick={() => setOpen((value) => !value)}
            className="ef-focus-ring h-11 rounded-control border border-border-strong bg-surface px-md text-label-lg text-fg md:hidden"
          >
            {/* Ten giu nguyen o ca 2 trang thai — aria-expanded da truyen trang thai roi. */}
            {t("header.menuToggle")}
          </button>
        </div>
      </Container>

      {/* Mot <nav> duy nhat cho ca desktop va mobile: khong nhan doi danh sach link.
        *
        * LUU Y: o mobile khi dong, `hidden` la display:none nen <nav> KHONG nam trong
        * accessibility tree — dung nhu mong doi cho mot menu dang dong. Landmark dieu huong
        * chi ton tai o desktop va o mobile khi menu mo. Trang thai doc qua data-state va
        * aria-expanded cua nut toggle. */}
      <nav
        id={navId}
        aria-label={t("header.navLabel")}
        data-state={open ? "open" : "closed"}
        // Bam vao link trung route hien tai thi pathname khong doi -> effect theo pathname
        // khong chay -> menu nam do che trang. Dong ngay tai day.
        onClick={() => setOpen(false)}
        className={cn("border-t border-border md:border-t-0", open ? "block" : "hidden md:block")}
      >
        <Container className="flex flex-col gap-xs py-sm md:h-12 md:flex-row md:items-center md:py-0">
          {items.map((item) => (
            <NavLink key={item.href} item={item} active={item.href === current} />
          ))}
        </Container>
      </nav>
    </header>
  );
}
