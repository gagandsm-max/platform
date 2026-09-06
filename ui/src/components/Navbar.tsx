import React from 'react';

interface NavbarProps {
  tenant: string;
  onTenantChange: (tenant: string) => void;
  env: string;
}

export const Navbar: React.FC<NavbarProps> = ({ tenant, onTenantChange, env }) => {
  return (
    <header className="platform-header">
      <div className="header-brand">
        <span className="brand-logo">Platform</span>
        <span className="badge env-badge">{env}</span>
      </div>
      <div className="header-controls">
        <div className="tenant-selector">
          <label htmlFor="tenant-select">Tenant:</label>
          <select
            id="tenant-select"
            value={tenant}
            onChange={(e) => onTenantChange(e.target.value)}
          >
            <option value="default">Default Tenant</option>
            <option value="enterprise-corp">Enterprise Corp</option>
            <option value="system">System (Internal)</option>
          </select>
        </div>
        <div className="header-user">
          <span className="user-icon">👤</span>
          <span className="user-name">admin</span>
        </div>
      </div>
    </header>
  );
};
