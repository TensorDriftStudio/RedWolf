import React, { useState, useEffect, useCallback } from 'react';
import type { 
  SystemSettings, 
  DirectoryTestResult, 
  OperatingSystem, 
  CloudInitTemplate, 
  OSImageInfo, 
  VersionInfo,
  User,
  UserRole
} from '../types';
import { 
  Download, 
  CheckCircle2, 
  HardDrive, 
  RefreshCw, 
  Loader2,
  UserPlus,
  Key,
  Trash2,
  Edit2,
  Shield,
  AlertTriangle
} from 'lucide-react';
import { getAuthHeaders } from '../utils/auth';


interface SettingsViewProps {
  onBackToFleet?: () => void;
  activeTab?: 'auth' | 'network' | 'storage' | 'templates';
  onTabChange?: (tab: 'auth' | 'network' | 'storage' | 'templates') => void;
  onSettingsSaved?: (newSettings: SystemSettings) => void;
}

const defaultLocalSettings: SystemSettings = {
  general: {
    applianceName: 'RedWolf Bare-Metal Appliance',
    serverUrl: 'http://192.168.0.250:8080',
    provisioningInterface: 'eth0',
    defaultOs: 'AlmaLinux 9',
  },
  network: {
    subnetCidr: '192.168.0.0/24',
    dhcpRangeStart: '192.168.0.100',
    dhcpRangeEnd: '192.168.0.200',
    gateway: '192.168.0.1',
    dnsServers: ['1.1.1.1', '8.8.8.8'],
    leaseDurationMinutes: 1440,
  },
  auth: {
    localAuthEnabled: true,
    ldap: {
      enabled: false,
      host: 'ldap.corp.example.com',
      port: 389,
      useTls: false,
      startTls: true,
      insecureSkipVerify: false,
      bindDn: 'cn=readonly,dc=corp,dc=example,dc=com',
      bindPassword: '',
      baseDn: 'dc=corp,dc=example,dc=com',
      userFilter: '(&(objectClass=posixAccount)(uid=%s))',
      groupSearchDn: 'ou=Groups,dc=corp,dc=example,dc=com',
      adminGroupDn: 'cn=RedWolf-Admins,ou=Groups,dc=corp,dc=example,dc=com',
      operatorGroupDn: 'cn=RedWolf-Operators,ou=Groups,dc=corp,dc=example,dc=com',
    },
    activeDirectory: {
      enabled: false,
      domain: 'corp.redwolf.internal',
      domainController: 'dc01.corp.redwolf.internal',
      port: 636,
      useLdaps: true,
      insecureSkipVerify: false,
      bindDn: 'svc-redwolf@corp.redwolf.internal',
      bindPassword: '',
      baseDn: 'DC=corp,DC=redwolf,DC=internal',
      userSearchFilter: '(&(objectClass=user)(|(sAMAccountName=%s)(userPrincipalName=%s)))',
      adminGroup: 'CN=Domain Admins,CN=Users,DC=corp,DC=redwolf,DC=internal',
      operatorGroup: 'CN=Server Operators,CN=Builtin,DC=corp,DC=redwolf,DC=internal',
    },
  },
  storage: {
    imageStorageDir: '/var/lib/redwolf/images',
    maxCacheSizeGb: 100,
  },
  updatedAt: new Date().toISOString(),
};

export const SettingsView: React.FC<SettingsViewProps> = ({ 
  activeTab: externalTab, 
  onTabChange,
  onBackToFleet,
  onSettingsSaved
}) => {
  const [internalTab, setInternalTab] = useState<'auth' | 'network' | 'storage' | 'templates'>('auth');
  const activeTab = externalTab || internalTab;
  const setActiveTab = (tab: 'auth' | 'network' | 'storage' | 'templates') => {
    setInternalTab(tab);
    if (onTabChange) onTabChange(tab);
  };
  const [settings, setSettings] = useState<SystemSettings>(defaultLocalSettings);
  const [dnsInput, setDnsInput] = useState<string>('1.1.1.1, 8.8.8.8');
  const [isLoadingSettings, setIsLoadingSettings] = useState<boolean>(true);
  const [isSaving, setIsSaving] = useState<boolean>(false);
  const [saveSuccess, setSaveSuccess] = useState<boolean>(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [versionInfo, setVersionInfo] = useState<VersionInfo | null>(null);

  // Fetch appliance version info
  useEffect(() => {
    fetch('/api/version')
      .then((res) => (res.ok ? res.json() : null))
      .then((data: VersionInfo | null) => {
        if (data) setVersionInfo(data);
      })
      .catch(() => {});
  }, []);

  // Cloud-Init Templates State
  const [templates, setTemplates] = useState<CloudInitTemplate[]>([]);
  const [editingTemplate, setEditingTemplate] = useState<CloudInitTemplate | null>(null);
  const [isTemplateModalOpen, setIsTemplateModalOpen] = useState<boolean>(false);
  const [templateFormError, setTemplateFormError] = useState<string | null>(null);

  const loadTemplates = () => {
    fetch('/api/templates', {
      headers: { ...getAuthHeaders() },
    })
      .then((res) => (res.ok ? res.json() : []))
      .then((data: CloudInitTemplate[]) => setTemplates(data))
      .catch(() => {});
  };

  useEffect(() => {
    loadTemplates();
  }, []);

  // OS Image Catalog State
  const [images, setImages] = useState<OSImageInfo[]>([]);
  const [isDownloadingImage, setIsDownloadingImage] = useState<string | null>(null);

  const loadImages = () => {
    fetch('/api/images', {
      headers: { ...getAuthHeaders() },
    })
      .then((res) => (res.ok ? res.json() : []))
      .then((data: OSImageInfo[]) => setImages(data))
      .catch(() => {});
  };

  useEffect(() => {
    loadImages();
  }, []);

  // Poll image status if any download or conversion is active
  useEffect(() => {
    const hasActiveDownload = images.some(
      (img) => img.downloadStatus?.status === 'downloading' || img.downloadStatus?.status === 'converting'
    );
    if (!hasActiveDownload && !isDownloadingImage) return;

    const interval = setInterval(() => {
      loadImages();
    }, 2000);

    return () => clearInterval(interval);
  }, [images, isDownloadingImage]);

  const handleDownloadImage = async (osType: OperatingSystem) => {
    setIsDownloadingImage(osType);
    try {
      const res = await fetch('/api/images/download', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...getAuthHeaders(),
        },
        body: JSON.stringify({ os: osType }),
      });
      if (res.ok) {
        loadImages();
      }
    } catch {
      // ignore
    } finally {
      setIsDownloadingImage(null);
    }
  };

  // Unified Directory Test State
  const [directoryType, setDirectoryType] = useState<'active_directory' | 'ldap'>('active_directory');
  const [isTestingDirectory, setIsTestingDirectory] = useState<boolean>(false);
  const [directoryTestResult, setDirectoryTestResult] = useState<DirectoryTestResult | null>(null);

  // Local User Accounts State
  const [users, setUsers] = useState<User[]>([]);
  const [isLoadingUsers, setIsLoadingUsers] = useState<boolean>(false);
  const [userModalMode, setUserModalMode] = useState<'create' | 'edit' | 'password' | 'delete' | null>(null);
  const [selectedUser, setSelectedUser] = useState<User | null>(null);
  const [userFormError, setUserFormError] = useState<string | null>(null);

  // Form Fields
  const [formUsername, setFormUsername] = useState<string>('');
  const [formDisplayName, setFormDisplayName] = useState<string>('');
  const [formEmail, setFormEmail] = useState<string>('');
  const [formRole, setFormRole] = useState<UserRole>('OPERATOR');
  const [formPassword, setFormPassword] = useState<string>('');
  const [formConfirmPassword, setFormConfirmPassword] = useState<string>('');

  const loadUsers = useCallback(() => {
    setIsLoadingUsers(true);
    fetch('/api/users', {
      headers: { ...getAuthHeaders() },
    })
      .then((res) => (res.ok ? res.json() : []))
      .then((data: User[]) => {
        if (Array.isArray(data)) setUsers(data);
      })
      .catch(() => {})
      .finally(() => setIsLoadingUsers(false));
  }, []);

  useEffect(() => {
    loadUsers();
  }, [loadUsers]);

  const handleOpenCreateUser = () => {
    setFormUsername('');
    setFormDisplayName('');
    setFormEmail('');
    setFormRole('OPERATOR');
    setFormPassword('');
    setFormConfirmPassword('');
    setUserFormError(null);
    setUserModalMode('create');
  };

  const handleOpenEditUser = (u: User) => {
    setSelectedUser(u);
    setFormUsername(u.username);
    setFormDisplayName(u.displayName || u.username);
    setFormEmail(u.email || '');
    setFormRole(u.role);
    setUserFormError(null);
    setUserModalMode('edit');
  };

  const handleOpenPasswordUser = (u: User) => {
    setSelectedUser(u);
    setFormPassword('');
    setFormConfirmPassword('');
    setUserFormError(null);
    setUserModalMode('password');
  };

  const handleOpenDeleteUser = (u: User) => {
    setSelectedUser(u);
    setUserFormError(null);
    setUserModalMode('delete');
  };

  const handleCreateUserSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setUserFormError(null);
    if (!formUsername.trim()) {
      setUserFormError('Username is required.');
      return;
    }
    if (formPassword.length < 8) {
      setUserFormError('Password must be at least 8 characters long.');
      return;
    }
    if (formPassword !== formConfirmPassword) {
      setUserFormError('Passwords do not match.');
      return;
    }

    try {
      const res = await fetch('/api/users', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...getAuthHeaders(),
        },
        body: JSON.stringify({
          username: formUsername.trim(),
          displayName: formDisplayName.trim() || formUsername.trim(),
          email: formEmail.trim(),
          role: formRole,
          password: formPassword,
        }),
      });

      if (!res.ok) {
        const err = await res.json().catch(() => ({ error: 'Failed to create user' }));
        throw new Error(err.error || 'Failed to create user');
      }

      setUserModalMode(null);
      loadUsers();
    } catch (err: unknown) {
      setUserFormError(err instanceof Error ? err.message : 'Failed to create user');
    }
  };

  const handleEditUserSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedUser) return;
    setUserFormError(null);

    try {
      const res = await fetch(`/api/users/${selectedUser.id}`, {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
          ...getAuthHeaders(),
        },
        body: JSON.stringify({
          displayName: formDisplayName.trim() || selectedUser.username,
          email: formEmail.trim(),
          role: formRole,
        }),
      });

      if (!res.ok) {
        const err = await res.json().catch(() => ({ error: 'Failed to update user' }));
        throw new Error(err.error || 'Failed to update user');
      }

      setUserModalMode(null);
      loadUsers();
    } catch (err: unknown) {
      setUserFormError(err instanceof Error ? err.message : 'Failed to update user');
    }
  };

  const handleChangePasswordSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedUser) return;
    setUserFormError(null);

    if (formPassword.length < 8) {
      setUserFormError('New password must be at least 8 characters long.');
      return;
    }
    if (formPassword !== formConfirmPassword) {
      setUserFormError('Passwords do not match.');
      return;
    }

    try {
      const res = await fetch(`/api/users/${selectedUser.id}/password`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...getAuthHeaders(),
        },
        body: JSON.stringify({ newPassword: formPassword }),
      });

      if (!res.ok) {
        const err = await res.json().catch(() => ({ error: 'Failed to change password' }));
        throw new Error(err.error || 'Failed to change password');
      }

      setUserModalMode(null);
    } catch (err: unknown) {
      setUserFormError(err instanceof Error ? err.message : 'Failed to change password');
    }
  };

  const handleDeleteUserSubmit = async () => {
    if (!selectedUser) return;
    setUserFormError(null);

    try {
      const res = await fetch(`/api/users/${selectedUser.id}`, {
        method: 'DELETE',
        headers: { ...getAuthHeaders() },
      });

      if (!res.ok) {
        const err = await res.json().catch(() => ({ error: 'Failed to delete user' }));
        throw new Error(err.error || 'Failed to delete user');
      }

      setUserModalMode(null);
      loadUsers();
    } catch (err: unknown) {
      setUserFormError(err instanceof Error ? err.message : 'Failed to delete user');
    }
  };

  // Load existing settings from API if available
  useEffect(() => {
    setIsLoadingSettings(true);
    fetch('/api/settings', {
      headers: { ...getAuthHeaders() },
    })
      .then((res) => {
        if (res.ok) return res.json();
        throw new Error('Not reachable');
      })
      .then((data: SystemSettings) => {
        setSettings(data);
        if (data.network?.dnsServers) {
          setDnsInput(data.network.dnsServers.join(', '));
        }
        if (data.auth?.ldap?.enabled && !data.auth?.activeDirectory?.enabled) {
          setDirectoryType('ldap');
        } else {
          setDirectoryType('active_directory');
        }
      })
      .catch(() => {
        // Fall back to default local settings
      })
      .finally(() => {
        setIsLoadingSettings(false);
      });
  }, []);

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsSaving(true);
    setSaveSuccess(false);

    const parsedDns = dnsInput.split(',').map((s) => s.trim()).filter(Boolean);
    const updated: SystemSettings = {
      ...settings,
      network: {
        ...settings.network,
        dnsServers: parsedDns,
      },
    };

    try {
      const res = await fetch('/api/settings', {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
          ...getAuthHeaders(),
        },
        body: JSON.stringify(updated),
      });
      if (res.ok) {
        setSaveSuccess(true);
        setSaveError(null);
        if (onSettingsSaved) {
          onSettingsSaved(updated);
        }
        setTimeout(() => setSaveSuccess(false), 4000);
      } else {
        const errData = await res.json().catch(() => ({ error: 'Failed saving configuration' }));
        setSaveError(errData.error || 'Failed saving configuration');
        setTimeout(() => setSaveError(null), 5000);
      }
    } catch {
      setSaveError('Network error: RedWolf appliance unreachable');
      setTimeout(() => setSaveError(null), 5000);
    } finally {
      setIsSaving(false);
    }
  };

  const handleTestDirectory = async () => {
    setIsTestingDirectory(true);
    setDirectoryTestResult(null);
    try {
      const res = await fetch('/api/settings/test-directory', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...getAuthHeaders(),
        },
        body: JSON.stringify({
          source: directoryType === 'active_directory' ? 'ACTIVE_DIRECTORY' : 'LDAP',
          ldap: settings.auth.ldap,
          activeDirectory: settings.auth.activeDirectory,
        }),
      });
      if (res.ok) {
        const data = await res.json();
        setDirectoryTestResult(data);
      } else {
        throw new Error('Server returned error');
      }
    } catch {
      setDirectoryTestResult({
        success: false,
        latencyMs: 0,
        message: directoryType === 'active_directory'
          ? 'Could not connect to Active Directory Domain Controller. Verify DNS and LDAPS port 636.'
          : 'Could not connect to configured LDAP server. Check host reachability and firewall port 389.',
        entriesFound: 0,
        testedAt: new Date().toISOString(),
      });
    } finally {
      setIsTestingDirectory(false);
    }
  };


  return (
    <div className="space-y-4 max-w-6xl mx-auto">
      {/* Title & Save Bar */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between pb-3 border-b border-[#212836] gap-3">
        <div className="flex items-center gap-2.5">
          {onBackToFleet && (
            <button
              type="button"
              onClick={onBackToFleet}
              className="text-xs text-slate-400 hover:text-white px-2 py-1 rounded bg-[#151b24] border border-[#212836] mr-1 transition-colors"
            >
              ← Back
            </button>
          )}
          <h2 className="text-base font-bold text-white">
            Settings
          </h2>
          <span className="font-mono text-[10px] text-slate-400 bg-[#151b24] border border-[#212836] px-1.5 py-0.2 rounded">
            {versionInfo?.version || 'v1.1.0'}
          </span>
          {versionInfo && (
            <span className="hidden md:inline text-slate-500 font-mono text-[10px]">
              {versionInfo.platform}
            </span>
          )}
        </div>
        <div className="flex items-center gap-3">
          {isLoadingSettings && (
            <span className="text-xs text-slate-400 flex items-center gap-1.5 font-medium">
              <Loader2 className="w-3.5 h-3.5 animate-spin text-redwolf-primary" />
              Syncing...
            </span>
          )}
          {saveError && (
            <span className="text-xs text-rose-400 flex items-center gap-1.5 font-medium">
              <AlertTriangle className="w-4 h-4 text-rose-400" />
              {saveError}
            </span>
          )}
          {saveSuccess && (
            <span className="text-xs text-emerald-400 flex items-center gap-1.5 font-medium">
              <CheckCircle2 className="w-4 h-4" />
              Settings Saved
            </span>
          )}
          <button
            type="button"
            onClick={handleSave}
            disabled={isSaving || isLoadingSettings}
            className="py-1.5 px-3.5 bg-redwolf-primary hover:bg-redwolf-hover disabled:bg-slate-800 text-white font-medium text-xs rounded-sm transition-colors flex items-center gap-2 shadow-sm"
          >
            {isSaving ? 'Saving...' : 'Save Configuration'}
          </button>
        </div>
      </div>

      {/* Navigation Tabs */}
      <div className="flex border-b border-[#212836] gap-1 text-xs">
        <button
          type="button"
          onClick={() => setActiveTab('auth')}
          className={`py-2 px-3 font-medium border-b-2 transition-colors ${
            activeTab === 'auth'
              ? 'border-redwolf-primary text-white bg-[#151b24]'
              : 'border-transparent text-slate-400 hover:text-slate-200'
          }`}
        >
          Authentication
        </button>
        <button
          type="button"
          onClick={() => setActiveTab('network')}
          className={`py-2 px-3 font-medium border-b-2 transition-colors ${
            activeTab === 'network'
              ? 'border-redwolf-primary text-white bg-[#151b24]'
              : 'border-transparent text-slate-400 hover:text-slate-200'
          }`}
        >
          Network & DHCP
        </button>
        <button
          type="button"
          onClick={() => setActiveTab('storage')}
          className={`py-2 px-3 font-medium border-b-2 transition-colors ${
            activeTab === 'storage'
              ? 'border-redwolf-primary text-white bg-[#151b24]'
              : 'border-transparent text-slate-400 hover:text-slate-200'
          }`}
        >
          OS Images & Storage
        </button>
        <button
          type="button"
          onClick={() => setActiveTab('templates')}
          className={`py-2 px-3 font-medium border-b-2 transition-colors ${
            activeTab === 'templates'
              ? 'border-redwolf-primary text-white bg-[#151b24]'
              : 'border-transparent text-slate-400 hover:text-slate-200'
          }`}
        >
          Templates ({templates.length})
        </button>
      </div>

      {/* Tab 1: Authentication & Directory Services */}
      {activeTab === 'auth' && (
        <div className="space-y-6">
          {/* Local Authentication & Operator Accounts Card */}
          <div className="bg-slate-900 border border-slate-800 rounded-sm p-5 space-y-5">
            <div className="flex flex-col sm:flex-row sm:items-center justify-between pb-3 border-b border-slate-800 gap-3">
              <div>
                <div className="flex items-center gap-2">
                  <Shield className="w-4 h-4 text-red-500" />
                  <h3 className="text-xs font-semibold text-slate-200">
                    Local Authentication & Operator Accounts
                  </h3>
                </div>
                <p className="text-[11px] text-slate-400 mt-0.5">
                  Manage local database operator identities, roles, and administrative passwords.
                </p>
              </div>

              <div className="flex items-center gap-4">
                <button
                  type="button"
                  onClick={handleOpenCreateUser}
                  className="flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium rounded-sm bg-redwolf-primary hover:bg-redwolf-hover text-white transition-colors shadow-sm"
                >
                  <UserPlus className="w-3.5 h-3.5" />
                  <span>Add Operator</span>
                </button>

                <div className="flex items-center gap-2 pl-3 border-l border-slate-800">
                  <span className="text-[11px] text-slate-400">Local Auth:</span>
                  <label className="relative inline-flex items-center cursor-pointer">
                    <input
                      type="checkbox"
                      checked={settings.auth.localAuthEnabled}
                      onChange={(e) =>
                        setSettings({
                          ...settings,
                          auth: { ...settings.auth, localAuthEnabled: e.target.checked },
                        })
                      }
                      className="sr-only peer"
                    />
                    <div className="w-9 h-5 bg-slate-800 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-slate-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-red-600"></div>
                  </label>
                </div>
              </div>
            </div>

            {/* Users Table */}
            <div className="overflow-x-auto border border-slate-800 rounded-sm">
              <table className="w-full text-left text-xs text-slate-300">
                <thead className="bg-[#121620] text-[11px] uppercase tracking-wider text-slate-400 border-b border-slate-800">
                  <tr>
                    <th className="py-2 px-3 font-semibold">User</th>
                    <th className="py-2 px-3 font-semibold">Role</th>
                    <th className="py-2 px-3 font-semibold">Email</th>
                    <th className="py-2 px-3 font-semibold">Created</th>
                    <th className="py-2 px-3 font-semibold">Last Login</th>
                    <th className="py-2 px-3 font-semibold text-right">Actions</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-800/60 font-mono text-[11px]">
                  {isLoadingUsers ? (
                    <tr>
                      <td colSpan={6} className="py-6 text-center text-slate-500">
                        <div className="flex items-center justify-center gap-2">
                          <Loader2 className="w-4 h-4 animate-spin text-redwolf-primary" />
                          <span>Loading operator accounts...</span>
                        </div>
                      </td>
                    </tr>
                  ) : users.length === 0 ? (
                    <tr>
                      <td colSpan={6} className="py-6 text-center text-slate-500">
                        No local users configured. Click "Add Operator" above to provision accounts.
                      </td>
                    </tr>
                  ) : (
                    users.map((u) => (
                      <tr key={u.id} className="hover:bg-slate-800/40 transition-colors">
                        <td className="py-2.5 px-3">
                          <div className="font-semibold text-slate-200">{u.displayName || u.username}</div>
                          <div className="text-[10px] text-slate-500 font-mono">@{u.username}</div>
                        </td>
                        <td className="py-2.5 px-3">
                          <span
                            className={`px-1.5 py-0.5 rounded text-[10px] font-semibold border ${
                              u.role === 'ADMIN'
                                ? 'bg-red-950/80 border-red-800 text-red-300'
                                : u.role === 'OPERATOR'
                                ? 'bg-amber-950/80 border-amber-800 text-amber-300'
                                : 'bg-slate-800 border-slate-700 text-slate-400'
                            }`}
                          >
                            {u.role}
                          </span>
                        </td>
                        <td className="py-2.5 px-3 text-slate-400 truncate max-w-xs">
                          {u.email || '—'}
                        </td>
                        <td className="py-2.5 px-3 text-slate-400">
                          {u.createdAt ? new Date(u.createdAt).toLocaleDateString() : '—'}
                        </td>
                        <td className="py-2.5 px-3 text-slate-400">
                          {u.lastLoginAt ? new Date(u.lastLoginAt).toLocaleString() : 'Never'}
                        </td>
                        <td className="py-2.5 px-3 text-right">
                          <div className="flex items-center justify-end gap-1 font-sans">
                            <button
                              type="button"
                              onClick={() => handleOpenPasswordUser(u)}
                              title="Change Password"
                              className="p-1 rounded text-slate-400 hover:text-amber-400 hover:bg-slate-800 transition-colors"
                            >
                              <Key className="w-3.5 h-3.5" />
                            </button>
                            <button
                              type="button"
                              onClick={() => handleOpenEditUser(u)}
                              title="Edit User"
                              className="p-1 rounded text-slate-400 hover:text-slate-100 hover:bg-slate-800 transition-colors"
                            >
                              <Edit2 className="w-3.5 h-3.5" />
                            </button>
                            <button
                              type="button"
                              onClick={() => handleOpenDeleteUser(u)}
                              title="Delete User"
                              className="p-1 rounded text-slate-400 hover:text-red-400 hover:bg-slate-800 transition-colors"
                            >
                              <Trash2 className="w-3.5 h-3.5" />
                            </button>
                          </div>
                        </td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
          </div>

          {/* Unified Enterprise Directory Services (LDAP / Active Directory) Card */}
          <div className="bg-slate-900 border border-slate-800 rounded-sm p-5 space-y-5">
            <div className="flex flex-col sm:flex-row sm:items-center justify-between pb-3 border-b border-slate-800 gap-3">
              <div className="flex items-center gap-3">
                <div className="w-8 h-8 rounded bg-cyan-950/60 border border-cyan-800/60 flex items-center justify-center text-cyan-400 font-bold text-xs">
                  DIR
                </div>
                <div>
                  <h3 className="text-xs font-semibold text-slate-200">
                    Enterprise Directory Services (LDAP / Active Directory)
                  </h3>
                  <p className="text-[11px] text-slate-400">
                    Authenticate data center operators against Microsoft Active Directory (LDAPS) or RFC 4511 OpenLDAP / FreeIPA.
                  </p>
                </div>
              </div>

              {/* Master Directory Switch */}
              <div className="flex items-center gap-2">
                <span className="text-[11px] text-slate-400">Directory Auth:</span>
                <label className="relative inline-flex items-center cursor-pointer">
                  <input
                    type="checkbox"
                    checked={settings.auth.activeDirectory.enabled || settings.auth.ldap.enabled}
                    onChange={(e) => {
                      const enabled = e.target.checked;
                      setSettings({
                        ...settings,
                        auth: {
                          ...settings.auth,
                          activeDirectory: {
                            ...settings.auth.activeDirectory,
                            enabled: enabled && directoryType === 'active_directory',
                          },
                          ldap: {
                            ...settings.auth.ldap,
                            enabled: enabled && directoryType === 'ldap',
                          },
                        },
                      });
                    }}
                    className="sr-only peer"
                  />
                  <div className="w-9 h-5 bg-slate-800 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-slate-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-red-600"></div>
                </label>
              </div>
            </div>

            {/* Provider Type Selector Pills */}
            <div className="flex items-center gap-2 border-b border-slate-800/80 pb-3">
              <span className="text-xs font-medium text-slate-400 mr-2">Provider Type:</span>
              <button
                type="button"
                onClick={() => {
                  setDirectoryType('active_directory');
                  const currentlyActive = settings.auth.activeDirectory.enabled || settings.auth.ldap.enabled;
                  if (currentlyActive) {
                    setSettings({
                      ...settings,
                      auth: {
                        ...settings.auth,
                        activeDirectory: { ...settings.auth.activeDirectory, enabled: true },
                        ldap: { ...settings.auth.ldap, enabled: false },
                      },
                    });
                  }
                }}
                className={`py-1 px-3 text-xs rounded-sm font-medium transition-colors border ${
                  directoryType === 'active_directory'
                    ? 'bg-cyan-950/70 border-cyan-700 text-cyan-200'
                    : 'bg-[#151b24] border-slate-800 text-slate-400 hover:text-slate-200'
                }`}
              >
                Microsoft Active Directory (LDAPS)
              </button>

              <button
                type="button"
                onClick={() => {
                  setDirectoryType('ldap');
                  const currentlyActive = settings.auth.activeDirectory.enabled || settings.auth.ldap.enabled;
                  if (currentlyActive) {
                    setSettings({
                      ...settings,
                      auth: {
                        ...settings.auth,
                        activeDirectory: { ...settings.auth.activeDirectory, enabled: false },
                        ldap: { ...settings.auth.ldap, enabled: true },
                      },
                    });
                  }
                }}
                className={`py-1 px-3 text-xs rounded-sm font-medium transition-colors border ${
                  directoryType === 'ldap'
                    ? 'bg-blue-950/70 border-blue-700 text-blue-200'
                    : 'bg-[#151b24] border-slate-800 text-slate-400 hover:text-slate-200'
                }`}
              >
                OpenLDAP / FreeIPA (RFC 4511)
              </button>
            </div>

            {/* Active Directory Configuration Fields */}
            {directoryType === 'active_directory' && (
              <div className="space-y-4">
                <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                  <div>
                    <label className="block text-xs font-medium text-slate-300 mb-1">Domain Controller Host</label>
                    <input
                      type="text"
                      value={settings.auth.activeDirectory.domainController}
                      onChange={(e) =>
                        setSettings({
                          ...settings,
                          auth: {
                            ...settings.auth,
                            activeDirectory: {
                              ...settings.auth.activeDirectory,
                              domainController: e.target.value,
                            },
                          },
                        })
                      }
                      placeholder="dc01.corp.redwolf.internal"
                      className="w-full bg-[#0c0e14] border border-slate-800 rounded px-3 py-1.5 text-xs text-white focus:outline-none focus:border-red-600 font-mono"
                    />
                  </div>

                  <div>
                    <label className="block text-xs font-medium text-slate-300 mb-1">Active Directory Domain</label>
                    <input
                      type="text"
                      value={settings.auth.activeDirectory.domain}
                      onChange={(e) =>
                        setSettings({
                          ...settings,
                          auth: {
                            ...settings.auth,
                            activeDirectory: {
                              ...settings.auth.activeDirectory,
                              domain: e.target.value,
                            },
                          },
                        })
                      }
                      placeholder="corp.redwolf.internal"
                      className="w-full bg-[#0c0e14] border border-slate-800 rounded px-3 py-1.5 text-xs text-white focus:outline-none focus:border-red-600 font-mono"
                    />
                  </div>

                  <div>
                    <label className="block text-xs font-medium text-slate-300 mb-1">LDAPS Port</label>
                    <input
                      type="number"
                      value={settings.auth.activeDirectory.port}
                      onChange={(e) =>
                        setSettings({
                          ...settings,
                          auth: {
                            ...settings.auth,
                            activeDirectory: {
                              ...settings.auth.activeDirectory,
                              port: parseInt(e.target.value, 10) || 636,
                            },
                          },
                        })
                      }
                      className="w-full bg-[#0c0e14] border border-slate-800 rounded px-3 py-1.5 text-xs text-white focus:outline-none focus:border-red-600 font-mono"
                    />
                  </div>
                </div>

                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div>
                    <label className="block text-xs font-medium text-slate-300 mb-1">Service Account Bind DN / UPN</label>
                    <input
                      type="text"
                      value={settings.auth.activeDirectory.bindDn}
                      onChange={(e) =>
                        setSettings({
                          ...settings,
                          auth: {
                            ...settings.auth,
                            activeDirectory: {
                              ...settings.auth.activeDirectory,
                              bindDn: e.target.value,
                            },
                          },
                        })
                      }
                      placeholder="svc-redwolf@corp.redwolf.internal"
                      className="w-full bg-[#0c0e14] border border-slate-800 rounded px-3 py-1.5 text-xs text-white focus:outline-none focus:border-red-600 font-mono"
                    />
                  </div>

                  <div>
                    <label className="block text-xs font-medium text-slate-300 mb-1">Service Account Password</label>
                    <input
                      type="password"
                      value={settings.auth.activeDirectory.bindPassword || ''}
                      onChange={(e) =>
                        setSettings({
                          ...settings,
                          auth: {
                            ...settings.auth,
                            activeDirectory: {
                              ...settings.auth.activeDirectory,
                              bindPassword: e.target.value,
                            },
                          },
                        })
                      }
                      placeholder="••••••••••••"
                      className="w-full bg-[#0c0e14] border border-slate-800 rounded px-3 py-1.5 text-xs text-white focus:outline-none focus:border-red-600 font-mono"
                    />
                  </div>
                </div>

                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div>
                    <label className="block text-xs font-medium text-slate-300 mb-1">Base Search DN</label>
                    <input
                      type="text"
                      value={settings.auth.activeDirectory.baseDn}
                      onChange={(e) =>
                        setSettings({
                          ...settings,
                          auth: {
                            ...settings.auth,
                            activeDirectory: {
                              ...settings.auth.activeDirectory,
                              baseDn: e.target.value,
                            },
                          },
                        })
                      }
                      placeholder="DC=corp,DC=redwolf,DC=internal"
                      className="w-full bg-[#0c0e14] border border-slate-800 rounded px-3 py-1.5 text-xs text-white focus:outline-none focus:border-red-600 font-mono"
                    />
                  </div>

                  <div>
                    <label className="block text-xs font-medium text-slate-300 mb-1">User Search Filter</label>
                    <input
                      type="text"
                      value={settings.auth.activeDirectory.userSearchFilter}
                      onChange={(e) =>
                        setSettings({
                          ...settings,
                          auth: {
                            ...settings.auth,
                            activeDirectory: {
                              ...settings.auth.activeDirectory,
                              userSearchFilter: e.target.value,
                            },
                          },
                        })
                      }
                      placeholder="(&(objectClass=user)(|(sAMAccountName=%s)(userPrincipalName=%s)))"
                      className="w-full bg-[#0c0e14] border border-slate-800 rounded px-3 py-1.5 text-xs text-white focus:outline-none focus:border-red-600 font-mono"
                    />
                  </div>
                </div>

                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div>
                    <label className="block text-xs font-medium text-slate-300 mb-1">Administrator Security Group</label>
                    <input
                      type="text"
                      value={settings.auth.activeDirectory.adminGroup}
                      onChange={(e) =>
                        setSettings({
                          ...settings,
                          auth: {
                            ...settings.auth,
                            activeDirectory: {
                              ...settings.auth.activeDirectory,
                              adminGroup: e.target.value,
                            },
                          },
                        })
                      }
                      placeholder="CN=Domain Admins,CN=Users,DC=corp,DC=redwolf,DC=internal"
                      className="w-full bg-[#0c0e14] border border-slate-800 rounded px-3 py-1.5 text-xs text-white focus:outline-none focus:border-red-600 font-mono"
                    />
                  </div>

                  <div>
                    <label className="block text-xs font-medium text-slate-300 mb-1">Operator Security Group</label>
                    <input
                      type="text"
                      value={settings.auth.activeDirectory.operatorGroup}
                      onChange={(e) =>
                        setSettings({
                          ...settings,
                          auth: {
                            ...settings.auth,
                            activeDirectory: {
                              ...settings.auth.activeDirectory,
                              operatorGroup: e.target.value,
                            },
                          },
                        })
                      }
                      placeholder="CN=Server Operators,CN=Builtin,DC=corp,DC=redwolf,DC=internal"
                      className="w-full bg-[#0c0e14] border border-slate-800 rounded px-3 py-1.5 text-xs text-white focus:outline-none focus:border-red-600 font-mono"
                    />
                  </div>
                </div>

                <div className="flex items-center gap-2 pt-1">
                  <input
                    type="checkbox"
                    id="ad-skip-verify"
                    checked={settings.auth.activeDirectory.insecureSkipVerify}
                    onChange={(e) =>
                      setSettings({
                        ...settings,
                        auth: {
                          ...settings.auth,
                          activeDirectory: {
                            ...settings.auth.activeDirectory,
                            insecureSkipVerify: e.target.checked,
                          },
                        },
                      })
                    }
                    className="rounded bg-slate-800 border-slate-700 text-red-600 focus:ring-0"
                  />
                  <label htmlFor="ad-skip-verify" className="text-xs text-slate-400">
                    Skip TLS Certificate Verification (Allow self-signed Active Directory CA certificates)
                  </label>
                </div>
              </div>
            )}

            {/* OpenLDAP Configuration Fields */}
            {directoryType === 'ldap' && (
              <div className="space-y-4">
                <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                  <div>
                    <label className="block text-xs font-medium text-slate-300 mb-1">LDAP Server Host</label>
                    <input
                      type="text"
                      value={settings.auth.ldap.host}
                      onChange={(e) =>
                        setSettings({
                          ...settings,
                          auth: {
                            ...settings.auth,
                            ldap: { ...settings.auth.ldap, host: e.target.value },
                          },
                        })
                      }
                      placeholder="ldap.corp.example.com"
                      className="w-full bg-[#0c0e14] border border-slate-800 rounded px-3 py-1.5 text-xs text-white focus:outline-none focus:border-red-600 font-mono"
                    />
                  </div>

                  <div>
                    <label className="block text-xs font-medium text-slate-300 mb-1">Port</label>
                    <input
                      type="number"
                      value={settings.auth.ldap.port}
                      onChange={(e) =>
                        setSettings({
                          ...settings,
                          auth: {
                            ...settings.auth,
                            ldap: { ...settings.auth.ldap, port: parseInt(e.target.value, 10) || 389 },
                          },
                        })
                      }
                      className="w-full bg-[#0c0e14] border border-slate-800 rounded px-3 py-1.5 text-xs text-white focus:outline-none focus:border-red-600 font-mono"
                    />
                  </div>

                  <div>
                    <label className="block text-xs font-medium text-slate-300 mb-1">Security Mode</label>
                    <select
                      value={settings.auth.ldap.useTls ? 'ldaps' : settings.auth.ldap.startTls ? 'starttls' : 'plain'}
                      onChange={(e) => {
                        const mode = e.target.value;
                        setSettings({
                          ...settings,
                          auth: {
                            ...settings.auth,
                            ldap: {
                              ...settings.auth.ldap,
                              useTls: mode === 'ldaps',
                              startTls: mode === 'starttls',
                              port: mode === 'ldaps' ? 636 : 389,
                            },
                          },
                        });
                      }}
                      className="w-full bg-[#0c0e14] border border-slate-800 rounded px-3 py-1.5 text-xs text-white focus:outline-none focus:border-red-600"
                    >
                      <option value="starttls">StartTLS (Port 389 - Recommended)</option>
                      <option value="ldaps">LDAPS (Port 636)</option>
                      <option value="plain">Plain LDAP (Insecure)</option>
                    </select>
                  </div>
                </div>

                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div>
                    <label className="block text-xs font-medium text-slate-300 mb-1">Service Account Bind DN</label>
                    <input
                      type="text"
                      value={settings.auth.ldap.bindDn}
                      onChange={(e) =>
                        setSettings({
                          ...settings,
                          auth: {
                            ...settings.auth,
                            ldap: { ...settings.auth.ldap, bindDn: e.target.value },
                          },
                        })
                      }
                      placeholder="cn=readonly,dc=corp,dc=example,dc=com"
                      className="w-full bg-[#0c0e14] border border-slate-800 rounded px-3 py-1.5 text-xs text-white focus:outline-none focus:border-red-600 font-mono"
                    />
                  </div>

                  <div>
                    <label className="block text-xs font-medium text-slate-300 mb-1">Service Account Password</label>
                    <input
                      type="password"
                      value={settings.auth.ldap.bindPassword || ''}
                      onChange={(e) =>
                        setSettings({
                          ...settings,
                          auth: {
                            ...settings.auth,
                            ldap: { ...settings.auth.ldap, bindPassword: e.target.value },
                          },
                        })
                      }
                      placeholder="••••••••••••"
                      className="w-full bg-[#0c0e14] border border-slate-800 rounded px-3 py-1.5 text-xs text-white focus:outline-none focus:border-red-600 font-mono"
                    />
                  </div>
                </div>

                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div>
                    <label className="block text-xs font-medium text-slate-300 mb-1">Base Search DN</label>
                    <input
                      type="text"
                      value={settings.auth.ldap.baseDn}
                      onChange={(e) =>
                        setSettings({
                          ...settings,
                          auth: {
                            ...settings.auth,
                            ldap: { ...settings.auth.ldap, baseDn: e.target.value },
                          },
                        })
                      }
                      placeholder="dc=corp,dc=example,dc=com"
                      className="w-full bg-[#0c0e14] border border-slate-800 rounded px-3 py-1.5 text-xs text-white focus:outline-none focus:border-red-600 font-mono"
                    />
                  </div>

                  <div>
                    <label className="block text-xs font-medium text-slate-300 mb-1">User Search Filter</label>
                    <input
                      type="text"
                      value={settings.auth.ldap.userFilter}
                      onChange={(e) =>
                        setSettings({
                          ...settings,
                          auth: {
                            ...settings.auth,
                            ldap: { ...settings.auth.ldap, userFilter: e.target.value },
                          },
                        })
                      }
                      placeholder="(&(objectClass=posixAccount)(uid=%s))"
                      className="w-full bg-[#0c0e14] border border-slate-800 rounded px-3 py-1.5 text-xs text-white focus:outline-none focus:border-red-600 font-mono"
                    />
                  </div>
                </div>

                <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                  <div>
                    <label className="block text-xs font-medium text-slate-300 mb-1">Group Search Base DN</label>
                    <input
                      type="text"
                      value={settings.auth.ldap.groupSearchDn}
                      onChange={(e) =>
                        setSettings({
                          ...settings,
                          auth: {
                            ...settings.auth,
                            ldap: { ...settings.auth.ldap, groupSearchDn: e.target.value },
                          },
                        })
                      }
                      placeholder="ou=Groups,dc=corp,dc=example,dc=com"
                      className="w-full bg-[#0c0e14] border border-slate-800 rounded px-3 py-1.5 text-xs text-white focus:outline-none focus:border-red-600 font-mono"
                    />
                  </div>

                  <div>
                    <label className="block text-xs font-medium text-slate-300 mb-1">Admin Group DN</label>
                    <input
                      type="text"
                      value={settings.auth.ldap.adminGroupDn}
                      onChange={(e) =>
                        setSettings({
                          ...settings,
                          auth: {
                            ...settings.auth,
                            ldap: { ...settings.auth.ldap, adminGroupDn: e.target.value },
                          },
                        })
                      }
                      placeholder="cn=RedWolf-Admins,ou=Groups,dc=..."
                      className="w-full bg-[#0c0e14] border border-slate-800 rounded px-3 py-1.5 text-xs text-white focus:outline-none focus:border-red-600 font-mono"
                    />
                  </div>

                  <div>
                    <label className="block text-xs font-medium text-slate-300 mb-1">Operator Group DN</label>
                    <input
                      type="text"
                      value={settings.auth.ldap.operatorGroupDn}
                      onChange={(e) =>
                        setSettings({
                          ...settings,
                          auth: {
                            ...settings.auth,
                            ldap: { ...settings.auth.ldap, operatorGroupDn: e.target.value },
                          },
                        })
                      }
                      placeholder="cn=RedWolf-Operators,ou=Groups,dc=..."
                      className="w-full bg-[#0c0e14] border border-slate-800 rounded px-3 py-1.5 text-xs text-white focus:outline-none focus:border-red-600 font-mono"
                    />
                  </div>
                </div>

                <div className="flex items-center gap-2 pt-1">
                  <input
                    type="checkbox"
                    id="ldap-skip-verify"
                    checked={settings.auth.ldap.insecureSkipVerify}
                    onChange={(e) =>
                      setSettings({
                        ...settings,
                        auth: {
                          ...settings.auth,
                          ldap: {
                            ...settings.auth.ldap,
                            insecureSkipVerify: e.target.checked,
                          },
                        },
                      })
                    }
                    className="rounded bg-slate-800 border-slate-700 text-red-600 focus:ring-0"
                  />
                  <label htmlFor="ldap-skip-verify" className="text-xs text-slate-400">
                    Skip TLS Certificate Verification (Allow self-signed LDAP server certificates)
                  </label>
                </div>
              </div>
            )}

            {/* Test Connection Button & Result Banner */}
            <div className="pt-2 border-t border-slate-800/80 flex flex-col sm:flex-row sm:items-center gap-3">
              <button
                type="button"
                onClick={handleTestDirectory}
                disabled={isTestingDirectory}
                className="py-1.5 px-3 bg-slate-800 hover:bg-slate-700 text-slate-200 font-medium text-xs rounded transition-colors flex items-center justify-center gap-2 border border-slate-700 disabled:opacity-50"
              >
                {isTestingDirectory ? (
                  <>
                    <Loader2 className="animate-spin h-3.5 w-3.5 text-slate-300" />
                    <span>Testing Directory Connection...</span>
                  </>
                ) : (
                  <span>Test Directory Connection ({directoryType === 'active_directory' ? 'Active Directory' : 'OpenLDAP'})</span>
                )}
              </button>

              {directoryTestResult && (
                <div
                  className={`text-xs px-3 py-1.5 rounded border flex items-center gap-2 ${
                    directoryTestResult.success
                      ? 'bg-emerald-950/60 border-emerald-800 text-emerald-300'
                      : 'bg-red-950/60 border-red-800 text-red-300'
                  }`}
                >
                  <span className="font-semibold">{directoryTestResult.success ? 'Success' : 'Failed'}</span>
                  <span>({directoryTestResult.latencyMs} ms)</span>
                  <span>— {directoryTestResult.message}</span>
                </div>
              )}
            </div>
          </div>

          {/* User Management Modals */}

          {/* Create User Modal */}
          {userModalMode === 'create' && (
            <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-xs p-4">
              <div className="bg-[#121620] border border-[#232b3b] shadow-2xl rounded-sm w-full max-w-md p-5 space-y-4">
                <div className="flex items-center justify-between pb-2 border-b border-slate-800">
                  <div className="flex items-center gap-2">
                    <UserPlus className="w-4 h-4 text-red-500" />
                    <h4 className="text-xs font-semibold text-slate-200">Create Local Operator</h4>
                  </div>
                  <button
                    type="button"
                    onClick={() => setUserModalMode(null)}
                    className="text-slate-400 hover:text-white text-xs"
                  >
                    ✕
                  </button>
                </div>

                {userFormError && (
                  <div className="p-2.5 bg-red-950/60 border border-red-800 text-red-300 text-xs rounded-sm">
                    {userFormError}
                  </div>
                )}

                <form onSubmit={handleCreateUserSubmit} className="space-y-3">
                  <div>
                    <label className="block text-[11px] font-medium text-slate-300 mb-1">
                      Username <span className="text-red-400">*</span>
                    </label>
                    <input
                      type="text"
                      autoFocus
                      value={formUsername}
                      onChange={(e) => setFormUsername(e.target.value)}
                      placeholder="e.g. jdoe"
                      className="w-full bg-[#0c0e14] border border-[#232b3b] rounded-sm px-3 py-1.5 text-xs text-white focus:outline-none focus:border-red-600 font-mono"
                    />
                  </div>

                  <div>
                    <label className="block text-[11px] font-medium text-slate-300 mb-1">
                      Display Name
                    </label>
                    <input
                      type="text"
                      value={formDisplayName}
                      onChange={(e) => setFormDisplayName(e.target.value)}
                      placeholder="e.g. John Doe"
                      className="w-full bg-[#0c0e14] border border-[#232b3b] rounded-sm px-3 py-1.5 text-xs text-white focus:outline-none focus:border-red-600"
                    />
                  </div>

                  <div>
                    <label className="block text-[11px] font-medium text-slate-300 mb-1">
                      Email Address
                    </label>
                    <input
                      type="email"
                      value={formEmail}
                      onChange={(e) => setFormEmail(e.target.value)}
                      placeholder="operator@datacenter.net"
                      className="w-full bg-[#0c0e14] border border-[#232b3b] rounded-sm px-3 py-1.5 text-xs text-white focus:outline-none focus:border-red-600"
                    />
                  </div>

                  <div>
                    <label className="block text-[11px] font-medium text-slate-300 mb-1">
                      Access Role <span className="text-red-400">*</span>
                    </label>
                    <select
                      value={formRole}
                      onChange={(e) => setFormRole(e.target.value as UserRole)}
                      className="w-full bg-[#0c0e14] border border-[#232b3b] rounded-sm px-3 py-1.5 text-xs text-white focus:outline-none focus:border-red-600"
                    >
                      <option value="ADMIN">ADMIN — Full root appliance privileges</option>
                      <option value="OPERATOR">OPERATOR — Provisioning, power actions & node inspection</option>
                      <option value="VIEWER">VIEWER — Read-only hardware telemetry inspection</option>
                    </select>
                  </div>

                  <div className="grid grid-cols-2 gap-2">
                    <div>
                      <label className="block text-[11px] font-medium text-slate-300 mb-1">
                        Password <span className="text-red-400">*</span>
                      </label>
                      <input
                        type="password"
                        value={formPassword}
                        onChange={(e) => setFormPassword(e.target.value)}
                        placeholder="••••••••••••"
                        className="w-full bg-[#0c0e14] border border-[#232b3b] rounded-sm px-3 py-1.5 text-xs text-white focus:outline-none focus:border-red-600 font-mono"
                      />
                    </div>
                    <div>
                      <label className="block text-[11px] font-medium text-slate-300 mb-1">
                        Confirm Password <span className="text-red-400">*</span>
                      </label>
                      <input
                        type="password"
                        value={formConfirmPassword}
                        onChange={(e) => setFormConfirmPassword(e.target.value)}
                        placeholder="••••••••••••"
                        className="w-full bg-[#0c0e14] border border-[#232b3b] rounded-sm px-3 py-1.5 text-xs text-white focus:outline-none focus:border-red-600 font-mono"
                      />
                    </div>
                  </div>

                  <div className="pt-3 border-t border-slate-800 flex justify-end gap-2">
                    <button
                      type="button"
                      onClick={() => setUserModalMode(null)}
                      className="px-3 py-1.5 text-xs rounded-sm bg-slate-800 hover:bg-slate-700 text-slate-300 transition-colors"
                    >
                      Cancel
                    </button>
                    <button
                      type="submit"
                      className="px-3 py-1.5 text-xs rounded-sm bg-redwolf-primary hover:bg-redwolf-hover text-white font-medium transition-colors"
                    >
                      Create Account
                    </button>
                  </div>
                </form>
              </div>
            </div>
          )}

          {/* Edit User Modal */}
          {userModalMode === 'edit' && selectedUser && (
            <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-xs p-4">
              <div className="bg-[#121620] border border-[#232b3b] shadow-2xl rounded-sm w-full max-w-md p-5 space-y-4">
                <div className="flex items-center justify-between pb-2 border-b border-slate-800">
                  <div className="flex items-center gap-2">
                    <Edit2 className="w-4 h-4 text-slate-300" />
                    <h4 className="text-xs font-semibold text-slate-200">
                      Edit User: @{selectedUser.username}
                    </h4>
                  </div>
                  <button
                    type="button"
                    onClick={() => setUserModalMode(null)}
                    className="text-slate-400 hover:text-white text-xs"
                  >
                    ✕
                  </button>
                </div>

                {userFormError && (
                  <div className="p-2.5 bg-red-950/60 border border-red-800 text-red-300 text-xs rounded-sm">
                    {userFormError}
                  </div>
                )}

                <form onSubmit={handleEditUserSubmit} className="space-y-3">
                  <div>
                    <label className="block text-[11px] font-medium text-slate-300 mb-1">
                      Display Name
                    </label>
                    <input
                      type="text"
                      autoFocus
                      value={formDisplayName}
                      onChange={(e) => setFormDisplayName(e.target.value)}
                      className="w-full bg-[#0c0e14] border border-[#232b3b] rounded-sm px-3 py-1.5 text-xs text-white focus:outline-none focus:border-red-600"
                    />
                  </div>

                  <div>
                    <label className="block text-[11px] font-medium text-slate-300 mb-1">
                      Email Address
                    </label>
                    <input
                      type="email"
                      value={formEmail}
                      onChange={(e) => setFormEmail(e.target.value)}
                      className="w-full bg-[#0c0e14] border border-[#232b3b] rounded-sm px-3 py-1.5 text-xs text-white focus:outline-none focus:border-red-600"
                    />
                  </div>

                  <div>
                    <label className="block text-[11px] font-medium text-slate-300 mb-1">
                      Access Role
                    </label>
                    <select
                      value={formRole}
                      onChange={(e) => setFormRole(e.target.value as UserRole)}
                      className="w-full bg-[#0c0e14] border border-[#232b3b] rounded-sm px-3 py-1.5 text-xs text-white focus:outline-none focus:border-red-600"
                    >
                      <option value="ADMIN">ADMIN — Full root appliance privileges</option>
                      <option value="OPERATOR">OPERATOR — Provisioning, power actions & node inspection</option>
                      <option value="VIEWER">VIEWER — Read-only hardware telemetry inspection</option>
                    </select>
                  </div>

                  <div className="pt-3 border-t border-slate-800 flex justify-end gap-2">
                    <button
                      type="button"
                      onClick={() => setUserModalMode(null)}
                      className="px-3 py-1.5 text-xs rounded-sm bg-slate-800 hover:bg-slate-700 text-slate-300 transition-colors"
                    >
                      Cancel
                    </button>
                    <button
                      type="submit"
                      className="px-3 py-1.5 text-xs rounded-sm bg-redwolf-primary hover:bg-redwolf-hover text-white font-medium transition-colors"
                    >
                      Save Changes
                    </button>
                  </div>
                </form>
              </div>
            </div>
          )}

          {/* Change Password Modal */}
          {userModalMode === 'password' && selectedUser && (
            <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-xs p-4">
              <div className="bg-[#121620] border border-[#232b3b] shadow-2xl rounded-sm w-full max-w-sm p-5 space-y-4">
                <div className="flex items-center justify-between pb-2 border-b border-slate-800">
                  <div className="flex items-center gap-2">
                    <Key className="w-4 h-4 text-amber-400" />
                    <h4 className="text-xs font-semibold text-slate-200">
                      Reset Password: @{selectedUser.username}
                    </h4>
                  </div>
                  <button
                    type="button"
                    onClick={() => setUserModalMode(null)}
                    className="text-slate-400 hover:text-white text-xs"
                  >
                    ✕
                  </button>
                </div>

                {userFormError && (
                  <div className="p-2.5 bg-red-950/60 border border-red-800 text-red-300 text-xs rounded-sm">
                    {userFormError}
                  </div>
                )}

                <form onSubmit={handleChangePasswordSubmit} className="space-y-3">
                  <div>
                    <label className="block text-[11px] font-medium text-slate-300 mb-1">
                      New Password (min 8 characters)
                    </label>
                    <input
                      type="password"
                      autoFocus
                      value={formPassword}
                      onChange={(e) => setFormPassword(e.target.value)}
                      placeholder="••••••••••••"
                      className="w-full bg-[#0c0e14] border border-[#232b3b] rounded-sm px-3 py-1.5 text-xs text-white focus:outline-none focus:border-red-600 font-mono"
                    />
                  </div>

                  <div>
                    <label className="block text-[11px] font-medium text-slate-300 mb-1">
                      Confirm New Password
                    </label>
                    <input
                      type="password"
                      value={formConfirmPassword}
                      onChange={(e) => setFormConfirmPassword(e.target.value)}
                      placeholder="••••••••••••"
                      className="w-full bg-[#0c0e14] border border-[#232b3b] rounded-sm px-3 py-1.5 text-xs text-white focus:outline-none focus:border-red-600 font-mono"
                    />
                  </div>

                  <div className="pt-3 border-t border-slate-800 flex justify-end gap-2">
                    <button
                      type="button"
                      onClick={() => setUserModalMode(null)}
                      className="px-3 py-1.5 text-xs rounded-sm bg-slate-800 hover:bg-slate-700 text-slate-300 transition-colors"
                    >
                      Cancel
                    </button>
                    <button
                      type="submit"
                      className="px-3 py-1.5 text-xs rounded-sm bg-redwolf-primary hover:bg-redwolf-hover text-white font-medium transition-colors"
                    >
                      Update Password
                    </button>
                  </div>
                </form>
              </div>
            </div>
          )}

          {/* Delete User Confirmation Modal */}
          {userModalMode === 'delete' && selectedUser && (
            <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-xs p-4">
              <div className="bg-[#121620] border border-[#232b3b] shadow-2xl rounded-sm w-full max-w-sm p-5 space-y-4">
                <div className="flex items-center gap-2 pb-2 border-b border-slate-800 text-red-400">
                  <AlertTriangle className="w-4 h-4" />
                  <h4 className="text-xs font-semibold text-slate-200">
                    Delete Operator Account
                  </h4>
                </div>

                {userFormError && (
                  <div className="p-2.5 bg-red-950/60 border border-red-800 text-red-300 text-xs rounded-sm">
                    {userFormError}
                  </div>
                )}

                <p className="text-xs text-slate-300 leading-relaxed">
                  Are you sure you want to permanently delete operator account{' '}
                  <span className="font-semibold text-white font-mono">@{selectedUser.username}</span>? This action cannot be reversed.
                </p>

                <div className="pt-2 border-t border-slate-800 flex justify-end gap-2">
                  <button
                    type="button"
                    onClick={() => setUserModalMode(null)}
                    className="px-3 py-1.5 text-xs rounded-sm bg-slate-800 hover:bg-slate-700 text-slate-300 transition-colors"
                  >
                    Cancel
                  </button>
                  <button
                    type="button"
                    onClick={handleDeleteUserSubmit}
                    className="px-3 py-1.5 text-xs rounded-sm bg-red-700 hover:bg-red-600 text-white font-medium transition-colors"
                  >
                    Confirm Delete
                  </button>
                </div>
              </div>
            </div>
          )}
        </div>
      )}

      {/* Tab 2: PXE Engine & Network */}
      {activeTab === 'network' && (
        <div className="bg-slate-900 border border-slate-800 rounded-sm p-5 space-y-5">
          <div className="pb-3 border-b border-slate-800">
            <h3 className="text-xs font-semibold text-slate-200">
              PXE Engine & DHCP Subnet Parameters
            </h3>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label className="block text-xs font-medium text-slate-300 mb-1">Server URL (Advertised)</label>
              <input
                type="text"
                value={settings.general.serverUrl}
                onChange={(e) =>
                  setSettings({
                    ...settings,
                    general: { ...settings.general, serverUrl: e.target.value },
                  })
                }
                className="w-full bg-slate-950 border border-slate-700 rounded-sm px-3 py-1.5 text-xs text-slate-100 font-mono focus:outline-none focus:border-red-600"
              />
            </div>

            <div>
              <label className="block text-xs font-medium text-slate-300 mb-1">Provisioning Interface</label>
              <input
                type="text"
                value={settings.general.provisioningInterface}
                onChange={(e) =>
                  setSettings({
                    ...settings,
                    general: { ...settings.general, provisioningInterface: e.target.value },
                  })
                }
                className="w-full bg-slate-950 border border-slate-700 rounded-sm px-3 py-1.5 text-xs text-slate-100 font-mono focus:outline-none focus:border-red-600"
              />
            </div>

            <div>
              <label className="block text-xs font-medium text-slate-300 mb-1">
                Subnet CIDR (e.g. 192.168.50.0/24)
              </label>
              <input
                type="text"
                value={settings.network.subnetCidr}
                onChange={(e) =>
                  setSettings({
                    ...settings,
                    network: { ...settings.network, subnetCidr: e.target.value },
                  })
                }
                className="w-full bg-slate-950 border border-slate-700 rounded-sm px-3 py-1.5 text-xs text-slate-100 font-mono focus:outline-none focus:border-red-600"
              />
            </div>

            <div>
              <label className="block text-xs font-medium text-slate-300 mb-1">Default Gateway</label>
              <input
                type="text"
                value={settings.network.gateway}
                onChange={(e) => {
                  const gw = e.target.value;
                  const parts = gw.trim().split('.');
                  let newSubnet = settings.network.subnetCidr;
                  if (parts.length === 4 && parts.slice(0, 3).every((p) => p !== '' && !isNaN(Number(p)) && Number(p) >= 0 && Number(p) <= 255)) {
                    if (settings.network.subnetCidr === '192.168.0.0/24' || !settings.network.subnetCidr) {
                      newSubnet = `${parts[0]}.${parts[1]}.${parts[2]}.0/24`;
                    }
                  }
                  setSettings({
                    ...settings,
                    network: { ...settings.network, gateway: gw, subnetCidr: newSubnet },
                  });
                }}
                className="w-full bg-slate-950 border border-slate-700 rounded-sm px-3 py-1.5 text-xs text-slate-100 font-mono focus:outline-none focus:border-red-600"
              />
            </div>

            <div>
              <label className="block text-xs font-medium text-slate-300 mb-1">DHCP Range Start</label>
              <input
                type="text"
                value={settings.network.dhcpRangeStart}
                onChange={(e) => {
                  const val = e.target.value;
                  const parts = val.trim().split('.');
                  let newSubnet = settings.network.subnetCidr;
                  if (parts.length === 4 && parts.slice(0, 3).every((p) => p !== '' && !isNaN(Number(p)) && Number(p) >= 0 && Number(p) <= 255)) {
                    if (settings.network.subnetCidr === '192.168.0.0/24' || !settings.network.subnetCidr) {
                      newSubnet = `${parts[0]}.${parts[1]}.${parts[2]}.0/24`;
                    }
                  }
                  setSettings({
                    ...settings,
                    network: { ...settings.network, dhcpRangeStart: val, subnetCidr: newSubnet },
                  });
                }}
                className="w-full bg-slate-950 border border-slate-700 rounded-sm px-3 py-1.5 text-xs text-slate-100 font-mono focus:outline-none focus:border-red-600"
              />
            </div>

            <div>
              <label className="block text-xs font-medium text-slate-300 mb-1">DHCP Range End</label>
              <input
                type="text"
                value={settings.network.dhcpRangeEnd}
                onChange={(e) =>
                  setSettings({
                    ...settings,
                    network: { ...settings.network, dhcpRangeEnd: e.target.value },
                  })
                }
                className="w-full bg-slate-950 border border-slate-700 rounded-sm px-3 py-1.5 text-xs text-slate-100 font-mono focus:outline-none focus:border-red-600"
              />
            </div>

            <div>
              <label className="block text-xs font-medium text-slate-300 mb-1">DNS Nameservers (comma-separated)</label>
              <input
                type="text"
                value={dnsInput}
                onChange={(e) => setDnsInput(e.target.value)}
                className="w-full bg-slate-950 border border-slate-700 rounded-sm px-3 py-1.5 text-xs text-slate-100 font-mono focus:outline-none focus:border-red-600"
              />
            </div>
          </div>
        </div>
      )}

      {/* Tab 3: Distribution Mirror & Cache */}
      {activeTab === 'storage' && (
        <div className="bg-slate-900 border border-slate-800 rounded-sm p-5 space-y-5">
          <div className="pb-3 border-b border-slate-800">
            <h3 className="text-xs font-semibold text-slate-200">
              OS Distribution Images & Storage
            </h3>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label className="block text-xs font-medium text-slate-300 mb-1">Image Storage Directory</label>
              <input
                type="text"
                value={settings.storage.imageStorageDir}
                onChange={(e) =>
                  setSettings({
                    ...settings,
                    storage: { ...settings.storage, imageStorageDir: e.target.value },
                  })
                }
                className="w-full bg-slate-950 border border-slate-700 rounded-sm px-3 py-1.5 text-xs text-slate-100 font-mono focus:outline-none focus:border-red-600"
              />
            </div>

            <div>
              <label className="block text-xs font-medium text-slate-300 mb-1">Default Operating System</label>
              <select
                value={settings.general.defaultOs}
                onChange={(e) =>
                  setSettings({
                    ...settings,
                    general: { ...settings.general, defaultOs: e.target.value as OperatingSystem },
                  })
                }
                className="w-full bg-slate-950 border border-slate-700 rounded-sm px-3 py-1.5 text-xs text-slate-100 focus:outline-none focus:border-red-600"
              >
                <option value="AlmaLinux 9">AlmaLinux 9 (Recommended - Current Enterprise)</option>
                <option value="AlmaLinux 8">AlmaLinux 8 (Legacy Enterprise)</option>
                <option value="Debian 12">Debian 12 (Bookworm LTS)</option>
                <option value="AlmaLinux 10">AlmaLinux 10 (Preview)</option>
                <option value="Debian 13">Debian 13 (Trixie Preview)</option>
              </select>
            </div>
          </div>

          {/* OS Distribution Catalog Table */}
          <div className="pt-3 border-t border-slate-800 space-y-3">
            <div className="flex items-center justify-between">
              <div>
                <h4 className="text-xs font-bold text-slate-200 uppercase tracking-wider flex items-center gap-1.5">
                  <HardDrive className="h-3.5 w-3.5 text-red-500" />
                  <span>Local Distribution Image Cache</span>
                </h4>
                <p className="text-[11px] text-slate-400">
                  Pre-cached generic cloud raw images (.raw.zstd) streamed to bare-metal servers during zero-touch deployment.
                </p>
              </div>
              <button
                type="button"
                onClick={() => loadImages()}
                className="px-2.5 py-1 bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs font-medium rounded border border-slate-700 transition-colors flex items-center gap-1.5"
              >
                <RefreshCw className="h-3 w-3" />
                <span>Refresh Cache</span>
              </button>
            </div>

            <div className="overflow-x-auto rounded border border-slate-800 bg-slate-950">
              <table className="w-full text-left text-xs">
                <thead>
                  <tr className="border-b border-slate-800 bg-slate-900/80 text-[11px] font-semibold text-slate-400 uppercase tracking-wider">
                    <th className="py-2.5 px-3">Distribution</th>
                    <th className="py-2.5 px-3">Raw Image Filename</th>
                    <th className="py-2.5 px-3">Cache Size</th>
                    <th className="py-2.5 px-3">Status</th>
                    <th className="py-2.5 px-3 text-right">Actions</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-800/60">
                  {images.map((img) => {
                    const isDownloading =
                      img.downloadStatus?.status === 'downloading' ||
                      img.downloadStatus?.status === 'converting' ||
                      isDownloadingImage === img.os;
                    const progress = img.downloadStatus?.progress || 0;

                    return (
                      <tr key={img.os} className="hover:bg-slate-900/40 transition-colors">
                        <td className="py-2.5 px-3 font-semibold text-slate-200">
                          {img.displayName}
                        </td>
                        <td className="py-2.5 px-3 font-mono text-[11px] text-slate-400">
                          {img.filename}
                        </td>
                        <td className="py-2.5 px-3 font-mono text-slate-300">
                          {img.present ? (
                            <span>{(img.sizeBytes / (1024 * 1024 * 1024)).toFixed(2)} GB</span>
                          ) : isDownloading && img.downloadStatus?.copiedBytes ? (
                            <span>{(img.downloadStatus.copiedBytes / (1024 * 1024)).toFixed(1)} MB</span>
                          ) : (
                            <span className="text-slate-500">—</span>
                          )}
                        </td>
                        <td className="py-2.5 px-3">
                          {img.present ? (
                            <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded text-[10px] font-bold bg-emerald-950/80 text-emerald-400 border border-emerald-800/60">
                              <CheckCircle2 className="h-3 w-3" />
                              <span>CACHED</span>
                            </span>
                          ) : isDownloading ? (
                            <div className="space-y-1">
                              <span className="inline-flex items-center gap-1 text-[11px] font-semibold text-amber-400">
                                <RefreshCw className="h-3 w-3 animate-spin" />
                                <span>{img.downloadStatus?.status === 'converting' ? `Converting (${progress}%)` : `Downloading (${progress}%)`}</span>
                              </span>
                              <div className="w-28 h-1.5 bg-slate-800 rounded-full overflow-hidden">
                                <div
                                  className="h-full bg-red-600 transition-all duration-300"
                                  style={{ width: `${Math.max(5, progress)}%` }}
                                />
                              </div>
                            </div>
                          ) : img.downloadStatus?.status === 'error' ? (
                            <div className="space-y-0.5">
                              <span
                                className="inline-flex items-center gap-1 px-2 py-0.5 rounded text-[10px] font-bold bg-red-950/80 text-red-400 border border-red-800/60"
                                title={img.downloadStatus.error}
                              >
                                <AlertTriangle className="h-3 w-3" />
                                <span>FAILED</span>
                              </span>
                              {img.downloadStatus.error && (
                                <p className="text-[10px] text-red-400 max-w-[140px] truncate" title={img.downloadStatus.error}>
                                  {img.downloadStatus.error}
                                </p>
                              )}
                            </div>
                          ) : (
                            <span className="inline-flex items-center px-2 py-0.5 rounded text-[10px] font-medium bg-slate-800 text-slate-400 border border-slate-700">
                              NOT CACHED
                            </span>
                          )}
                        </td>
                        <td className="py-2.5 px-3 text-right">
                          {img.downloadUrl ? (
                            <button
                              type="button"
                              disabled={isDownloading}
                              onClick={() => handleDownloadImage(img.os)}
                              className={`px-2.5 py-1 rounded text-[11px] font-medium transition-colors inline-flex items-center gap-1 ${
                                img.present
                                  ? 'bg-slate-800 hover:bg-slate-700 text-slate-300 border border-slate-700'
                                  : 'bg-red-700 hover:bg-red-600 text-white'
                              } disabled:opacity-50`}
                            >
                              <Download className="h-3 w-3" />
                              <span>{img.present ? 'Re-cache' : 'Download'}</span>
                            </button>
                          ) : (
                            <span className="text-[11px] text-slate-500 italic">Custom Mirror Required</span>
                          )}
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      )}

      {/* Tab 4: Cloud-Init Templates */}
      {activeTab === 'templates' && (
        <div className="space-y-6">
          <div className="flex items-center justify-between pb-3 border-b border-slate-800">
            <div>
              <h3 className="text-xs font-semibold text-white">Cloud-Init Templates</h3>
            </div>
            <button
              type="button"
              onClick={() => {
                setEditingTemplate({
                  id: '',
                  name: '',
                  description: '',
                  distro: 'All',
                  userData: '#cloud-config\ngrowpart:\n  mode: auto\nresize_rootfs: true\n\npackage_update: true\npackages:\n  - curl\n  - htop\n',
                  isDefault: false,
                });
                setTemplateFormError(null);
                setIsTemplateModalOpen(true);
              }}
              className="py-1.5 px-3 bg-red-700 hover:bg-red-600 text-white text-xs font-medium rounded-sm transition-colors shadow-sm flex items-center gap-1.5"
            >
              <span>+ New Template</span>
            </button>
          </div>

          {/* Templates Grid */}
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            {templates.map((tpl) => (
              <div
                key={tpl.id}
                className="bg-slate-900 border border-slate-800 rounded-sm p-4 space-y-3 flex flex-col justify-between"
              >
                <div>
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-2">
                      <span className="font-bold text-white text-xs">{tpl.name}</span>
                      {tpl.isDefault && (
                        <span className="px-1.5 py-0.5 bg-emerald-950/80 border border-emerald-800 text-emerald-400 text-[10px] font-mono rounded">
                          DEFAULT
                        </span>
                      )}
                    </div>
                    <span className="text-[10px] font-mono px-1.5 py-0.5 bg-slate-800 text-slate-300 rounded">
                      {tpl.distro}
                    </span>
                  </div>
                  <p className="text-xs text-slate-400 mt-1 leading-relaxed">
                    {tpl.description}
                  </p>

                  <div className="mt-2.5">
                    <div className="text-[10px] text-slate-500 font-mono mb-1">USER-DATA PREVIEW:</div>
                    <pre className="bg-slate-950 p-2 rounded text-[10px] font-mono text-emerald-400/90 overflow-x-auto max-h-28 border border-slate-800/80">
                      {tpl.userData.slice(0, 180)}...
                    </pre>
                  </div>
                </div>

                <div className="pt-2 border-t border-slate-800/60 flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <button
                      type="button"
                      onClick={() => {
                        setEditingTemplate(tpl);
                        setTemplateFormError(null);
                        setIsTemplateModalOpen(true);
                      }}
                      className="text-xs text-slate-300 hover:text-white transition-colors"
                    >
                      Edit
                    </button>
                    {!tpl.isDefault && (
                      <button
                        type="button"
                        onClick={async () => {
                          await fetch(`/api/templates/${tpl.id}`, {
                            method: 'PUT',
                            headers: {
                              'Content-Type': 'application/json',
                              ...getAuthHeaders(),
                            },
                            body: JSON.stringify({ ...tpl, isDefault: true }),
                          });
                          loadTemplates();
                        }}
                        className="text-xs text-slate-400 hover:text-emerald-400 transition-colors"
                      >
                        Set Default
                      </button>
                    )}
                  </div>
                  <button
                    type="button"
                    onClick={async () => {
                      if (confirm(`Delete template "${tpl.name}"?`)) {
                        await fetch(`/api/templates/${tpl.id}`, {
                          method: 'DELETE',
                          headers: { ...getAuthHeaders() },
                        });
                        loadTemplates();
                      }
                    }}
                    className="text-xs text-red-500 hover:text-red-400 transition-colors"
                  >
                    Delete
                  </button>
                </div>
              </div>
            ))}
          </div>

          {/* Template Edit / Create Modal */}
          {isTemplateModalOpen && editingTemplate && (
            <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/75 p-4">
              <div className="bg-slate-900 border border-slate-700 rounded-sm w-full max-w-2xl p-5 space-y-4 shadow-2xl">
                <div className="flex items-center justify-between border-b border-slate-800 pb-3">
                  <h4 className="text-sm font-bold text-white">
                    {editingTemplate.id ? 'Edit Cloud-Init Template' : 'New Cloud-Init Template'}
                  </h4>
                  <button
                    type="button"
                    onClick={() => setIsTemplateModalOpen(false)}
                    className="text-slate-400 hover:text-white"
                  >
                    ✕
                  </button>
                </div>

                {templateFormError && (
                  <div className="p-2 bg-red-950/60 border border-red-800 text-red-300 text-xs rounded">
                    {templateFormError}
                  </div>
                )}

                <div className="space-y-3">
                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <label className="block text-xs font-medium text-slate-300 mb-1">Template Name</label>
                      <input
                        type="text"
                        value={editingTemplate.name}
                        onChange={(e) => setEditingTemplate({ ...editingTemplate, name: e.target.value })}
                        className="w-full bg-slate-950 border border-slate-700 rounded-sm px-3 py-1.5 text-xs text-slate-100 focus:outline-none focus:border-red-600"
                        placeholder="e.g. Ceph Storage Node"
                      />
                    </div>
                    <div>
                      <label className="block text-xs font-medium text-slate-300 mb-1">Target Distribution</label>
                      <select
                        value={editingTemplate.distro}
                        onChange={(e) => setEditingTemplate({ ...editingTemplate, distro: e.target.value })}
                        className="w-full bg-slate-950 border border-slate-700 rounded-sm px-3 py-1.5 text-xs text-slate-100 focus:outline-none focus:border-red-600"
                      >
                        <option value="All">All Supported Distributions</option>
                        <option value="AlmaLinux 9">AlmaLinux 9</option>
                        <option value="AlmaLinux 8">AlmaLinux 8</option>
                        <option value="Debian 12">Debian 12</option>
                      </select>
                    </div>
                  </div>

                  <div>
                    <label className="block text-xs font-medium text-slate-300 mb-1">Description</label>
                    <input
                      type="text"
                      value={editingTemplate.description}
                      onChange={(e) => setEditingTemplate({ ...editingTemplate, description: e.target.value })}
                      className="w-full bg-slate-950 border border-slate-700 rounded-sm px-3 py-1.5 text-xs text-slate-100 focus:outline-none focus:border-red-600"
                      placeholder="Brief summary of packages and configuration applied"
                    />
                  </div>

                  <div>
                    <label className="block text-xs font-medium text-slate-300 mb-1">
                      User-Data YAML (Must start with <code>#cloud-config</code>)
                    </label>
                    <textarea
                      rows={10}
                      value={editingTemplate.userData}
                      onChange={(e) => setEditingTemplate({ ...editingTemplate, userData: e.target.value })}
                      className="w-full bg-slate-950 border border-slate-700 rounded-sm p-3 text-xs font-mono text-emerald-400 focus:outline-none focus:border-red-600 leading-relaxed"
                    />
                  </div>

                  <div className="flex items-center gap-2">
                    <input
                      type="checkbox"
                      id="tpl-default"
                      checked={editingTemplate.isDefault}
                      onChange={(e) => setEditingTemplate({ ...editingTemplate, isDefault: e.target.checked })}
                      className="rounded border-slate-700 bg-slate-950 text-red-600 focus:ring-0"
                    />
                    <label htmlFor="tpl-default" className="text-xs text-slate-300">
                      Set as Default Template in Provisioning Wizard
                    </label>
                  </div>
                </div>

                <div className="flex items-center justify-end gap-2 pt-3 border-t border-slate-800">
                  <button
                    type="button"
                    onClick={() => setIsTemplateModalOpen(false)}
                    className="py-1.5 px-3 bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs rounded transition-colors"
                  >
                    Cancel
                  </button>
                  <button
                    type="button"
                    onClick={async () => {
                      if (!editingTemplate.name.trim()) {
                        setTemplateFormError('Template name is required');
                        return;
                      }
                      if (!editingTemplate.userData.trim().startsWith('#cloud-config')) {
                        setTemplateFormError('User-Data must begin with #cloud-config');
                        return;
                      }

                      const url = editingTemplate.id ? `/api/templates/${editingTemplate.id}` : '/api/templates';
                      const method = editingTemplate.id ? 'PUT' : 'POST';

                      try {
                        const res = await fetch(url, {
                          method,
                          headers: {
                            'Content-Type': 'application/json',
                            ...getAuthHeaders(),
                          },
                          body: JSON.stringify(editingTemplate),
                        });
                        if (!res.ok) {
                          const errData = await res.json();
                          setTemplateFormError(errData.error || 'Failed saving template');
                          return;
                        }
                        setIsTemplateModalOpen(false);
                        loadTemplates();
                      } catch (err: unknown) {
                        const msg = err instanceof Error ? err.message : 'Network error';
                        setTemplateFormError(msg);
                      }
                    }}
                    className="py-1.5 px-4 bg-red-700 hover:bg-red-600 text-white text-xs font-medium rounded transition-colors"
                  >
                    Save Template
                  </button>
                </div>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
};
