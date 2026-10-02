"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

import { Badge } from "@/components/ui";
import { cn } from "@/lib/cn";

import { isActive, type NavItem } from "./nav-config";

export interface NavLinkProps {
  item: NavItem;
  className?: string;
}

const BASE = "ef-focus-ring flex items-center gap-sm rounded-control px-sm py-sm text-label-lg";

export function NavLink({ item, className }: NavLinkProps) {
  const pathname = usePathname();

  // Route chua ton tai: KHONG render <a>. Mot link dan toi 404 te hon mot nhan noi
  // thang la chua co (handoff muc 5 nguyen tac 3).
  if (!item.ready) {
    return (
      <span aria-disabled="true" className={cn(BASE, "cursor-not-allowed text-fg-muted", className)}>
        {item.label}
        <Badge variant="neutral">Sắp có</Badge>
      </span>
    );
  }

  const active = isActive(pathname, item.href);

  return (
    <Link
      href={item.href}
      aria-current={active ? "page" : undefined}
      className={cn(
        BASE,
        active ? "bg-surface-subtle text-primary" : "text-fg hover:bg-surface-subtle",
        className,
      )}
    >
      {item.label}
    </Link>
  );
}
