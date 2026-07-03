import { ContentSection } from '../components/content-section'
import { AppearanceForm } from './appearance-form'

export function SettingsAppearance() {
  return (
    <ContentSection title='外观' desc='设置界面主题、字体和布局方向。'>
      <AppearanceForm />
    </ContentSection>
  )
}
