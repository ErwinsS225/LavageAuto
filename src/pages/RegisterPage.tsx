import AuthHeader from '@/components/auth/AuthHeader'
import RegisterForm from '@/components/auth/RegisterForm'
import { usePageTitle } from '@/hooks/usePageTitle'

/** Inscription (design LavPro, deux panneaux). */
export default function RegisterPage() {
  usePageTitle('Créer un compte')
  return (
    <>
      <AuthHeader />
      <RegisterForm />
    </>
  )
}
