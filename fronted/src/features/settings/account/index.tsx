import { ContentSection } from '../components/content-section'
import { AccountForm } from './account-form'

export function SettingsAccount() {
  return (
    <ContentSection title='账号安全' desc='修改当前账号的登录密码。'>
      <AccountForm />
    </ContentSection>
  )
}
