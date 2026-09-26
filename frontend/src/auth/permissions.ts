/** Permission names from backend/internal/auth/roles.go, limited to the ones the UI checks. */

export const Perm = {
  ProjectImport: 'project.import',
  VersionImport: 'version.import',
  ProjectDelete: 'project.delete',
  VersionDelete: 'version.delete',
  SettingsManage: 'settings.manage',
  UserManage: 'user.manage',
  ChatUse: 'chat.use',
} as const
export type Perm = (typeof Perm)[keyof typeof Perm]
