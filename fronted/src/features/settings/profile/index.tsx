import { ContentSection } from '../components/content-section'
import { ProfileForm } from './profile-form'

export function SettingsProfile() {
  return (
    <ContentSection title='个人资料' desc='修改当前账号在系统中展示的昵称。'>
      <ProfileForm />
    </ContentSection>
  )
}
