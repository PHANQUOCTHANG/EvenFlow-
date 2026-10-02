"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

import { Badge } from "@/components/ui";
import { cn } from "@/lib/cn";

import { isActive, type NavItem } from "./nav-config";

export interface NavLinkProps {
  item: NavItem;
  className?: string;
  /** Do danh sach cha quyet dinh (xem `activeHref`). Bo trong thi NavLink tu tinh bang
   *  `isActive` — chi dung duoc khi danh sach khong co item long tien to nhau. */
  active?: boolean;
}

const BASE = "ef-focus-ring flex items-center gap-sm rounded-control px-sm py-sm text-label-lg";

export function NavLink({ item, className, active: activeProp }: NavLinkProps) {
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

  const active = activeProp ?? isActive(pathname, item.href);

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
