import { ContentSection } from '../components/content-section'
import { AccountForm } from './account-form'

export function SettingsAccount() {
  return (
    <ContentSection
      title='Account'
      desc='Change the password used to sign in to your account.'
    >
      <AccountForm />
    </ContentSection>
  )
}
