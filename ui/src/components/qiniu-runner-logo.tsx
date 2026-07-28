import { Terminal } from "lucide-react"

export function QiniuRunnerLogo({ inverse = false }: { inverse?: boolean }) {
  return (
    <span data-brand-logo="qiniu-runner" className="flex items-center gap-2.5">
      <span className="rounded-lg bg-sidebar-primary p-1.5 shadow-sm">
        <Terminal className="h-[18px] w-[18px] text-sidebar-primary-foreground" />
      </span>
      <span className="flex flex-col">
        <span
          className={
            inverse
              ? "text-[17px] font-semibold leading-tight text-white"
              : "bg-gradient-to-r from-primary to-primary/70 bg-clip-text text-[17px] font-semibold leading-tight text-transparent"
          }
        >
          Qiniu
        </span>
        <span
          className={[
            "text-[11px] font-medium leading-none",
            inverse ? "text-white/60" : "text-muted-foreground",
          ].join(" ")}
        >
          CI Runner
        </span>
      </span>
    </span>
  )
}
