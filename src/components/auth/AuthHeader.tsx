import { Car } from 'lucide-react'

export function AuthHeader() {
  return (
    <div className="mb-6 flex items-center justify-center gap-2 md:justify-start">
      <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-[#47253F]/10">
        <Car size={18} className="text-[#47253F]" />
      </div>
      <span className="text-base font-semibold tracking-tight text-ink">LavPro</span>
    </div>
  )
}

export default AuthHeader
