import { useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useAuthStore } from '@/stores/auth-store'
import { useLayout } from '@/context/layout-provider'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarRail,
} from '@/components/ui/sidebar'
import { currentUserQueryOptions } from '@/features/auth/api'
import { isAdminUser } from '@/features/auth/permissions'
import { AppTitle } from './app-title'
import { getSidebarData } from './data/sidebar-data'
import { NavGroup } from './nav-group'
import { NavUser } from './nav-user'

export function AppSidebar() {
  const { collapsible, variant } = useLayout()
  const user = useAuthStore((state) => state.auth.user)
  const setUser = useAuthStore((state) => state.auth.setUser)
  const profileQuery = useQuery({
    ...currentUserQueryOptions(),
    initialData: user ?? undefined,
    refetchOnMount: 'always',
  })
  const currentUser = user ?? profileQuery.data
  const sidebarData = getSidebarData(isAdminUser(currentUser))

  useEffect(() => {
    if (profileQuery.data) setUser(profileQuery.data)
  }, [profileQuery.data, setUser])

  return (
    <Sidebar collapsible={collapsible} variant={variant}>
      <SidebarHeader>
        <AppTitle />
      </SidebarHeader>
      <SidebarContent>
        {sidebarData.navGroups.map((props) => (
          <NavGroup key={props.title} {...props} />
        ))}
      </SidebarContent>
      <SidebarFooter>
        <NavUser
          user={{
            displayName:
              currentUser?.nickname || currentUser?.username || '用户',
            username: currentUser?.username || '',
          }}
        />
      </SidebarFooter>
      <SidebarRail />
    </Sidebar>
  )
}
