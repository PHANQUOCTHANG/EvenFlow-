"use client";

import { usePathname } from "next/navigation";
import { useEffect, useId, useRef, useState, type ReactNode } from "react";

import { ThemeToggle } from "@/components/theme/theme-toggle";
import { cn } from "@/lib/cn";

import { Container } from "./container";
import type { NavItem } from "./nav-config";
import { NavLink } from "./nav-link";
import { SkipLink } from "./skip-link";

export interface WorkspaceShellProps {
  children: ReactNode;
  title: string;
  /** CAU HINH HIEN THI theo vai tro, KHONG phai quyen. Xem comment dau nav-config.ts. */
  items: NavItem[];
}

/** Shell cho (organizer) va (ops): sidebar 264px + topbar (handoff muc 6). */
export function WorkspaceShell({ children, title, items }: WorkspaceShellProps) {
  const pathname = usePathname();
  const [open, setOpen] = useState(false);
  const toggleRef = useRef<HTMLButtonElement>(null);
  const navId = useId();

  useEffect(() => {
    setOpen(false);
  }, [pathname]);

  useEffect(() => {
    if (!open) return;
    function onKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        setOpen(false);
        toggleRef.current?.focus();
      }
    }
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [open]);

  return (
    <>
      <SkipLink />
      <div className="min-h-screen md:grid md:grid-cols-[264px_1fr]">
        {/* Mot <nav> duy nhat, LUON trong accessibility tree. An tren mobile la viec cua CSS;
          * trang thai doc qua data-state va aria-expanded cua nut trong topbar. */}
        <nav
          id={navId}
          aria-label="Điều hướng workspace"
          data-state={open ? "open" : "closed"}
          className={cn(
            "border-border bg-surface md:border-r",
            open ? "block border-b" : "hidden md:block",
          )}
        >
          <Container className="flex flex-col gap-xs py-sm">
            {items.map((item) => (
              <NavLink key={item.href} item={item} />
            ))}
          </Container>
        </nav>

        <div className="min-w-0">
          <header className="border-b border-border bg-surface">
            <Container variant="workspace" className="flex h-16 items-center justify-between gap-md">
              <h1 className="text-headline-md text-fg">{title}</h1>
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
                  Mở menu điều hướng
                </button>
              </div>
            </Container>
          </header>

          <Container as="main" variant="workspace" id="main" tabIndex={-1} className="py-lg">
            {children}
          </Container>
        </div>
      </div>
    </>
  );
}
