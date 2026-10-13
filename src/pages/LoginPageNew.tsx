import { AuthHeader } from '@/components/auth/AuthHeader'
import LoginForm from '@/components/auth/LoginForm'
import { usePageTitle } from '@/hooks/usePageTitle'

/** Connexion (design LavPro, deux panneaux). */
export default function LoginPageNew() {
  usePageTitle('Connexion')
  return (
    <>
      <AuthHeader />
      <LoginForm />
    </>
  )
}
