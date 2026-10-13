import AuthHeader from '@/components/auth/AuthHeader'
import LoginForm from '@/components/auth/LoginForm'
import { usePageTitle } from '@/hooks/usePageTitle'

/** Connexion à l'espace d'une station (design LavPro, deux panneaux). */
export default function LoginPage() {
  usePageTitle('Connexion')
  return (
    <>
      <AuthHeader />
      <LoginForm />
    </>
  )
}
