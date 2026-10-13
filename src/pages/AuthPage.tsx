/**
 * Page d'authentification unifiée (connexion / inscription) avec split design.
 * Réutilise la même logique existante (useAuth, ApiError, redirections, toasts).
 */
import { useState } from 'react'

import AuthImagePanel from '@/layouts/AuthImagePanel'
import LoginForm from '@/components/auth/LoginForm'
import RegisterForm from '@/components/auth/RegisterForm'
import AuthHeader from '@/components/auth/AuthHeader'

export default function AuthPage() {
  const [mode, setMode] = useState<'login' | 'register'>('login')

  return (
    <div className="flex min-h-dvh items-center justify-center bg-[#F5F5F5] px-4 py-8">
      <div className="mx-auto w-full max-w-5xl overflow-hidden rounded-2xl border border-line/60 bg-surface shadow-[0_1px_2px_rgba(16,19,25,0.02),0_10px_40px_-20px_rgba(16,19,25,0.12)]">
        <div className="grid min-h-[640px] grid-cols-1 md:grid-cols-5">
          <div className="col-span-1 px-6 py-8 sm:px-8 md:col-span-2 md:px-10">
            <AuthHeader />
            {mode === 'login' ? (
              <>
                <LoginForm />
                <button
                  type="button"
                  onClick={() => setMode('register')}
                  className="sr-only"
                  aria-hidden="true"
                />
              </>
            ) : (
              <RegisterForm />
            )}
          </div>
          <div className="col-span-1 md:col-span-3">
            <AuthImagePanel />
          </div>
        </div>
      </div>
    </div>
  )
}
