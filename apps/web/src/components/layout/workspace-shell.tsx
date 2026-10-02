"use client";

import { usePathname } from "next/navigation";
import { useEffect, useId, useRef, useState, type ReactNode } from "react";

import { ThemeToggle } from "@/components/theme/theme-toggle";
import { cn } from "@/lib/cn";

import { Container } from "./container";
import { activeHref, type NavItem } from "./nav-config";
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

  const current = activeHref(items, pathname);

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
    <>
      <SkipLink />
      {/* Thu tu DOM: topbar -> nav -> main.
        *
        * Tren mobile day la block flow, nen menu mo ra NGAY DUOI nut toggle: Tab tu nut di
        * tiep vao cac link vua hien, va menu khong day topbar (voi ngon tay dang dat tren
        * nut) xuong duoi. Neu de <nav> truoc topbar thi ca hai dieu tren deu sai.
        *
        * Tren desktop grid dat lai vi tri: nav sang cot 1 va keo het 2 hang, topbar va main
        * o cot 2. Thu tu DOM khong doi. */}
      <div className="min-h-screen md:grid md:grid-cols-[264px_1fr] md:grid-rows-[auto_1fr]">
        <header className="border-b border-border bg-surface md:col-start-2 md:row-start-1">
          <Container variant="workspace" className="flex h-16 items-center justify-between gap-md">
            {/* <p> chu khong phai <h1>: day la ten khu vuc, do layout dat va giong nhau o moi
              * route trong group. <h1> thuoc ve page — neu shell chiem <h1> thi moi trang
              * organizer/ops deu co cung mot heading vo nghia, va page nao tu dat <h1> theo
              * dung pattern cua repo se tao ra <h1> thu hai. */}
            <p className="text-headline-md text-fg">{title}</p>
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

        {/* Mot <nav> duy nhat cho ca desktop va mobile, khong nhan doi danh sach link.
          *
          * LUU Y: o mobile khi dong, `hidden` la display:none nen <nav> KHONG nam trong
          * accessibility tree — dung nhu mong doi cho mot menu dang dong. Landmark dieu huong
          * chi ton tai o desktop va o mobile khi menu mo. */}
        <nav
          id={navId}
          aria-label="Điều hướng workspace"
          data-state={open ? "open" : "closed"}
          // Bam vao mot link trung route hien tai thi pathname khong doi, nen effect theo
          // pathname khong chay va menu se nam do che trang. Dong ngay tai day.
          onClick={() => setOpen(false)}
          className={cn(
            "border-border bg-surface md:col-start-1 md:row-start-1 md:row-span-2 md:border-r",
            open ? "block border-b md:border-b-0" : "hidden md:block",
          )}
        >
          <Container className="flex flex-col gap-xs py-sm">
            {items.map((item) => (
              <NavLink key={item.href} item={item} active={item.href === current} />
            ))}
          </Container>
        </nav>

        <Container
          as="main"
          variant="workspace"
          id="main"
          tabIndex={-1}
          className="py-lg md:col-start-2 md:row-start-2"
        >
          {children}
        </Container>
      </div>
    </>
  );
}
