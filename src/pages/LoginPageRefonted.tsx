/**
 * Wrapper pour conserver la logique existante tout en exposant la nouvelle vue.
 * On peut aussi remplacer directement LoginPage, mais cette approche est
 * non-invasive.
 */
import AuthPage from '@/pages/AuthPage'

export default function LoginPageRefonted() {
  return <AuthPage />
}
