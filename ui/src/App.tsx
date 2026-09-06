import React, { useState } from 'react';
import { Navbar } from './components/Navbar';
import { HealthStatus } from './components/HealthStatus';
import { ErrorBoundary } from './components/ErrorBoundary';
import './App.css';

export const App: React.FC = () => {
  const [activeTenant, setActiveTenant] = useState<string>('default');
  const [activeTab, setActiveTab] = useState<'overview' | 'infrastructure' | 'network' | 'analytics'>('overview');

  return (
    <ErrorBoundary>
      <div className="platform-container">
        <Navbar
          tenant={activeTenant}
          onTenantChange={setActiveTenant}
          env="DEV"
        />

        <div className="platform-body">
          <aside className="platform-sidebar">
            <nav className="nav-menu">
              <button
                className={`nav-item ${activeTab === 'overview' ? 'active' : ''}`}
                onClick={() => setActiveTab('overview')}
              >
                📊 Platform Overview
              </button>
              <button
                className={`nav-item ${activeTab === 'network' ? 'active' : ''}`}
                onClick={() => setActiveTab('network')}
              >
                🌐 Network Topology
              </button>
              <button
                className={`nav-item ${activeTab === 'infrastructure' ? 'active' : ''}`}
                onClick={() => setActiveTab('infrastructure')}
              >
                🖥️ Infrastructure & Agents
              </button>
              <button
                className={`nav-item ${activeTab === 'analytics' ? 'active' : ''}`}
                onClick={() => setActiveTab('analytics')}
              >
                📈 Telemetry & Analytics
              </button>
            </nav>
          </aside>

          <main className="platform-content">
            <div className="content-banner">
              <h2>Platform Engineering Control Plane</h2>
              <p>
                Authoritative, high-scale network and infrastructure observability engine.
                Tenant context: <strong>{activeTenant}</strong>
              </p>
            </div>

            <HealthStatus />

            <div className="card architecture-card">
              <h3>Bootstrap Architecture State</h3>
              <div className="summary-list">
                <div className="summary-item">
                  <strong>Control Plane (Core):</strong> Active Spring Boot 3.3 runtime over Java 21.
                </div>
                <div className="summary-item">
                  <strong>Authoritative DB:</strong> PostgreSQL 16 with schema migrations initialized.
                </div>
                <div className="summary-item">
                  <strong>Telemetry Store:</strong> ClickHouse analytical database for high-volume time-series metrics.
                </div>
                <div className="summary-item">
                  <strong>Streaming Backbone:</strong> Apache Kafka KRaft internal broker.
                </div>
                <div className="summary-item">
                  <strong>Ephemeral Cache:</strong> Valkey 7 (Redis-compatible) cache.
                </div>
                <div className="summary-item">
                  <strong>Edge Sensors:</strong> Multi-collector Go engines with bounded disk spools.
                </div>
              </div>
            </div>
          </main>
        </div>
      </div>
    </ErrorBoundary>
  );
};

export default App;
