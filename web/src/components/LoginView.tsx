import React, { useState } from 'react';
import type { AuthSource } from '../types';

interface LoginViewProps {
  onLogin: (username: string, password: string, source: AuthSource) => Promise<unknown>;
  isLoading: boolean;
  error: string | null;
}

export const LoginView: React.FC<LoginViewProps> = ({ onLogin, isLoading, error }) => {
  const [source, setSource] = useState<AuthSource>('LOCAL');
  const [username, setUsername] = useState<string>('');
  const [password, setPassword] = useState<string>('');
  const [localError, setLocalError] = useState<string | null>(null);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLocalError(null);
    if (!username.trim() || !password) {
      setLocalError('Please enter both username and password.');
      return;
    }
    try {
      await onLogin(username.trim(), password, source);
    } catch (err: unknown) {
      if (err instanceof Error) {
        setLocalError(err.message);
      }
    }
  };

  return (
    <div className="min-h-screen bg-slate-950 flex flex-col justify-center items-center p-4">
      {/* Container Card */}
      <div className="w-full max-w-md bg-slate-900 border border-slate-800 shadow-2xl rounded-sm">
        {/* Header */}
        <div className="p-6 border-b border-slate-800 bg-slate-900/80 text-center">
          <div className="flex justify-center mb-3">
            <img 
              src="/assets/redwolf-icon.svg" 
              alt="RedWolf" 
              className="h-16 w-16 object-contain drop-shadow-[0_0_15px_rgba(166,25,46,0.6)]" 
            />
          </div>
          <h1 className="text-xl font-bold tracking-tight text-slate-100 uppercase">
            RedWolf GUI
          </h1>
          <p className="text-xs text-slate-400 mt-1">
            Bare-Metal & VM Automated Provisioning Platform
          </p>
        </div>

        {/* Directory Source Selector */}
        <div className="border-b border-slate-800 bg-slate-950/40 p-2">
          <div className="grid grid-cols-3 gap-1">
            <button
              type="button"
              onClick={() => setSource('LOCAL')}
              className={`py-2 px-2 text-xs font-medium rounded-sm border transition-colors ${
                source === 'LOCAL'
                  ? 'bg-slate-800 text-slate-100 border-slate-700 shadow-sm'
                  : 'bg-transparent text-slate-400 border-transparent hover:text-slate-200'
              }`}
            >
              Local Admin
            </button>
            <button
              type="button"
              onClick={() => setSource('LDAP')}
              className={`py-2 px-2 text-xs font-medium rounded-sm border transition-colors ${
                source === 'LDAP'
                  ? 'bg-slate-800 text-slate-100 border-slate-700 shadow-sm'
                  : 'bg-transparent text-slate-400 border-transparent hover:text-slate-200'
              }`}
            >
              OpenLDAP
            </button>
            <button
              type="button"
              onClick={() => setSource('ACTIVE_DIRECTORY')}
              className={`py-2 px-2 text-xs font-medium rounded-sm border transition-colors ${
                source === 'ACTIVE_DIRECTORY'
                  ? 'bg-slate-800 text-slate-100 border-slate-700 shadow-sm'
                  : 'bg-transparent text-slate-400 border-transparent hover:text-slate-200'
              }`}
            >
              Active Directory
            </button>
          </div>
        </div>

        {/* Form Body */}
        <form onSubmit={handleSubmit} className="p-6 space-y-4">
          {/* Identity Provider Notice */}
          <div className="text-xs bg-slate-950/60 p-2.5 rounded border border-slate-800 text-slate-400">
            {source === 'LOCAL' && (
              <span>Authenticating with built-in appliance database credentials.</span>
            )}
            {source === 'LDAP' && (
              <span>Authenticating against enterprise OpenLDAP / FreeIPA (RFC 4511).</span>
            )}
            {source === 'ACTIVE_DIRECTORY' && (
              <span>Authenticating against Windows Active Directory Domain Services.</span>
            )}
          </div>

          {(error || localError) && (
            <div className="p-3 bg-red-950/50 border border-red-800/80 rounded text-red-300 text-xs flex items-start gap-2">
              <svg className="w-4 h-4 text-red-400 shrink-0 mt-0.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
              </svg>
              <span>{error || localError}</span>
            </div>
          )}

          <div>
            <label className="block text-xs font-medium text-slate-300 uppercase tracking-wider mb-1">
              {source === 'ACTIVE_DIRECTORY' ? 'Username (sAMAccountName or UPN)' : 'Username'}
            </label>
            <input
              type="text"
              autoFocus
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              placeholder={source === 'ACTIVE_DIRECTORY' ? 'operator@corp.internal' : 'admin'}
              className="w-full bg-slate-950 border border-slate-700 rounded-sm px-3 py-2 text-sm text-slate-100 placeholder-slate-500 focus:outline-none focus:border-red-600 focus:ring-1 focus:ring-red-600 transition-colors"
            />
          </div>

          <div>
            <label className="block text-xs font-medium text-slate-300 uppercase tracking-wider mb-1">
              Password
            </label>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="••••••••••••"
              className="w-full bg-slate-950 border border-slate-700 rounded-sm px-3 py-2 text-sm text-slate-100 placeholder-slate-500 focus:outline-none focus:border-red-600 focus:ring-1 focus:ring-red-600 transition-colors"
            />
          </div>

          <button
            type="submit"
            disabled={isLoading}
            className="w-full mt-2 py-2.5 px-4 bg-red-700 hover:bg-red-600 disabled:bg-slate-800 disabled:text-slate-600 text-white font-medium text-sm rounded-sm transition-colors flex items-center justify-center gap-2 shadow-sm"
          >
            {isLoading ? (
              <>
                <svg className="animate-spin h-4 w-4 text-white" viewBox="0 0 24 24" fill="none">
                  <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                  <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
                </svg>
                <span>Authenticating...</span>
              </>
            ) : (
              <span>Sign In to RedWolf GUI</span>
            )}
          </button>
        </form>

        {/* Footer */}
        <div className="p-3 bg-slate-950/80 border-t border-slate-800/80 text-center text-[10px] text-slate-500">
          RedWolf GUI v1.1.0 • Enterprise Edition • Multi-Vendor HAL
        </div>
      </div>
    </div>
  );
};
