import Image from "next/image";
import Link from "next/link";

export default function AuthLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="min-h-screen w-full lg:grid lg:grid-cols-2 bg-surface">
      {/* Left panel - Branding (Hidden on mobile) */}
      <div className="hidden lg:flex relative bg-brand text-brand-fg overflow-hidden flex-col justify-between p-12">
        {/* Background ambient effect */}
        <div className="absolute inset-0 bg-[radial-gradient(ellipse_at_top_left,_var(--ef-primary)_0%,_transparent_60%)] opacity-30" />
        <div className="absolute inset-0 bg-[url('/logo.png')] bg-no-repeat bg-center bg-[length:120%] opacity-5 mix-blend-overlay" />
        
        <div className="relative z-10">
          <Link href="/" className="inline-flex items-center gap-3">
             <div className="flex size-12 items-center justify-center rounded-xl bg-white/10 backdrop-blur-md border border-white/20">
               <Image src="/logo.png" alt="EventFlow" width={32} height={32} className="object-contain" priority />
             </div>
             <span className="text-display-xs font-bold text-white tracking-tight">EventFlow</span>
          </Link>
        </div>

        <div className="relative z-10 w-full pr-8">
          <h1 className="text-display-sm font-bold text-white mb-5 leading-tight">
            Nền tảng phân phối vé công bằng & an toàn
          </h1>
          <p className="w-full text-body-lg text-brand-fg/80 mb-10 leading-relaxed">
            EventFlow loại trừ hành vi đầu cơ vé thông qua công nghệ định danh Zero-Trust. Trải nghiệm mua vé minh bạch, nhanh chóng và bảo mật.
          </p>
          <div className="flex items-center gap-2.5 text-body-sm font-medium text-brand-fg/80 bg-white/5 w-fit px-4 py-2.5 rounded-full border border-white/10 backdrop-blur-md shadow-sm">
             <svg className="size-4 text-primary-fg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
               <path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.8 17 5 19 5a1 1 0 0 1 1 1z" />
               <path d="m9 12 2 2 4-4" />
             </svg>
             <span>Bảo vệ bởi EventFlow Identity Guard</span>
          </div>
        </div>
      </div>

      {/* Right panel - Form */}
      <div className="w-full flex flex-col justify-center items-center p-6 sm:p-12 relative bg-bg">
        {/* Mobile Header (Hidden on Desktop) */}
        <div className="lg:hidden absolute top-0 left-0 right-0 p-6 flex items-center justify-between z-10">
           <Link href="/" className="ef-focus-ring inline-flex items-center gap-1.5 text-body-sm font-medium text-fg-muted hover:text-primary transition-colors bg-surface/50 backdrop-blur-md px-3 py-1.5 rounded-full border border-border">
            <svg className="size-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <line x1="19" y1="12" x2="5" y2="12" />
              <polyline points="12 19 5 12 12 5" />
            </svg>
            <span>Trang chủ</span>
          </Link>
        </div>
        
        {/* Mobile Logo Logo */}
        <div className="lg:hidden w-full max-w-[420px] flex items-center justify-center gap-2.5 mb-8 mt-12">
           <div className="flex size-10 items-center justify-center rounded-xl bg-surface border border-border shadow-sm">
             <Image src="/logo.png" alt="EventFlow" width={24} height={24} className="object-contain" priority />
           </div>
           <span className="text-display-xs font-bold text-fg tracking-tight">EventFlow</span>
        </div>

        <div className="w-full max-w-[420px] bg-surface sm:border sm:border-border sm:shadow-card sm:p-8 sm:rounded-card relative z-10">
          {children}
        </div>
      </div>
    </div>
  )
}
