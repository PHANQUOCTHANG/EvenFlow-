"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useId, useRef, useState } from "react";

import { ThemeToggle } from "@/components/theme/theme-toggle";
import { cn } from "@/lib/cn";

import { Container } from "./container";
import { PUBLIC_NAV, type NavItem } from "./nav-config";
import { NavLink } from "./nav-link";

export function PublicHeader({ items = PUBLIC_NAV }: { items?: NavItem[] }) {
  const pathname = usePathname();
  const [open, setOpen] = useState(false);
  const toggleRef = useRef<HTMLButtonElement>(null);
  const navId = useId();

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
      <Container className="flex h-16 items-center justify-between gap-md">
        <Link href="/" className="ef-focus-ring rounded-control text-headline-md text-fg">
          EventFlow
        </Link>

        <div className="flex items-center gap-sm">
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
            Mở menu điều hướng
          </button>
        </div>
      </Container>

      {/* Mot <nav> duy nhat cho ca desktop va mobile: khong nhan doi danh sach link.
        * KHONG dung thuoc tinh `hidden`/`aria-hidden` — landmark dieu huong phai luon ton tai
        * trong accessibility tree. An tren mobile la viec cua CSS; trang thai mo/dong doc
        * duoc qua data-state va aria-expanded cua nut. */}
      <nav
        id={navId}
        aria-label="Điều hướng chính"
        data-state={open ? "open" : "closed"}
        className={cn(
          "border-t border-border md:border-t-0",
          open ? "block" : "hidden md:block",
        )}
      >
        <Container className="flex flex-col gap-xs py-sm md:h-12 md:flex-row md:items-center md:py-0">
          {items.map((item) => (
            <NavLink key={item.href} item={item} />
          ))}
        </Container>
      </nav>
    </header>
  );
}
