import React, { useState, useEffect } from 'react';
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
  const [version, setVersion] = useState<string>('1.2.8');

  useEffect(() => {
    fetch('/api/version')
      .then((res) => (res.ok ? res.json() : null))
      .then((data: { version?: string } | null) => {
        if (data?.version) {
          setVersion(data.version.replace(/^v/, ''));
        }
      })
      .catch(() => {});
  }, []);

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
    <div className="min-h-screen bg-[#0b0e14] flex flex-col justify-center items-center p-4">
      {/* Container Card */}
      <div className="w-full max-w-sm bg-[#121620] border border-[#212836] shadow-xl rounded-sm">
        
        {/* Header */}
        <div className="p-6 border-b border-[#212836] text-center">
          <div className="flex justify-center mb-3">
            <img 
              src="/assets/redwolf-icon.svg" 
              alt="RedWolf" 
              className="h-12 w-12 object-contain" 
            />
          </div>
          <h1 className="text-lg font-bold tracking-tight text-white uppercase">
            RedWolf
          </h1>
          <p className="text-xs text-slate-400 mt-0.5">
            Bare-Metal Provisioning Engine
          </p>
        </div>

        {/* Authentication Source Selector */}
        <div className="border-b border-[#212836] bg-[#0c0e14] p-1.5">
          <div className="grid grid-cols-2 gap-1">
            <button
              type="button"
              onClick={() => setSource('LOCAL')}
              className={`py-1.5 px-2 text-xs font-medium rounded-sm border transition-colors ${
                source === 'LOCAL'
                  ? 'bg-[#181f2c] text-white border-[#2b3547]'
                  : 'bg-transparent text-slate-400 border-transparent hover:text-slate-200'
              }`}
            >
              Local Account
            </button>
            <button
              type="button"
              onClick={() => setSource('DIRECTORY')}
              className={`py-1.5 px-2 text-xs font-medium rounded-sm border transition-colors ${
                source === 'DIRECTORY'
                  ? 'bg-[#181f2c] text-white border-[#2b3547]'
                  : 'bg-transparent text-slate-400 border-transparent hover:text-slate-200'
              }`}
            >
              Directory (LDAP / AD)
            </button>
          </div>
        </div>

        {/* Form Body */}
        <form onSubmit={handleSubmit} className="p-5 space-y-3.5">
          {(error || localError) && (
            <div className="p-2.5 bg-red-950/60 border border-red-800 text-red-300 text-xs rounded-sm">
              {error || localError}
            </div>
          )}

          <div>
            <label className="block text-[11px] font-medium text-slate-300 mb-1">
              {source === 'DIRECTORY' ? 'Directory Username (or user@corp.domain)' : 'Username'}
            </label>
            <input
              type="text"
              autoFocus
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              placeholder={source === 'DIRECTORY' ? 'operator or operator@corp.internal' : 'admin'}
              className="w-full bg-[#0c0e14] border border-[#232b3b] rounded-sm px-3 py-1.5 text-xs text-white placeholder-slate-500 focus:outline-none focus:border-redwolf-primary transition-colors"
            />
          </div>


          <div>
            <label className="block text-[11px] font-medium text-slate-300 mb-1">
              Password
            </label>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="••••••••••••"
              className="w-full bg-[#0c0e14] border border-[#232b3b] rounded-sm px-3 py-1.5 text-xs text-white placeholder-slate-500 focus:outline-none focus:border-redwolf-primary transition-colors"
            />
          </div>

          <button
            type="submit"
            disabled={isLoading}
            className="w-full mt-2 py-2 px-4 bg-redwolf-primary hover:bg-redwolf-hover disabled:bg-slate-800 disabled:text-slate-500 text-white font-medium text-xs rounded-sm transition-colors flex items-center justify-center gap-2 shadow-sm"
          >
            {isLoading ? 'Signing in...' : 'Sign In'}
          </button>
        </form>

        {/* Footer */}
        <div className="py-2.5 px-4 bg-[#0c0e14] border-t border-[#212836] text-center text-[10px] text-slate-500 font-mono">
          RedWolf v{version}
        </div>
      </div>
    </div>
  );
};
