import React, { useEffect, useState } from 'react';

interface HealthData {
  status: string;
  version?: string;
  core?: string;
  components?: Record<string, { status: string; details?: any }>;
}

export const HealthStatus: React.FC = () => {
  const [health, setHealth] = useState<HealthData | null>(null);
  const [loading, setLoading] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);

  const fetchHealth = async () => {
    try {
      const resp = await fetch('/api/v1/health/connectivity');
      if (!resp.ok) {
        throw new Error(`HTTP error! status: ${resp.status}`);
      }
      const data = await resp.json();
      setHealth(data);
      setError(null);
    } catch (err: any) {
      setError(err.message || 'Failed to connect to Platform Core');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchHealth();
    const timer = setInterval(fetchHealth, 10000);
    return () => clearInterval(timer);
  }, []);

  if (loading && !health) {
    return <div className="card loading-card">Connecting to Platform Core...</div>;
  }

  return (
    <div className="card health-card">
      <div className="card-header">
        <h3>Platform Core & Telemetry Connectivity</h3>
        <button className="btn-refresh" onClick={fetchHealth}>Refresh</button>
      </div>

      {error ? (
        <div className="alert alert-danger">
          <strong>Connectivity Error:</strong> {error}
        </div>
      ) : (
        <div className="health-grid">
          <div className="health-tile">
            <span className="tile-label">Platform Core</span>
            <span className="badge badge-success">{health?.status || 'UP'}</span>
          </div>
          <div className="health-tile">
            <span className="tile-label">Version</span>
            <span className="badge badge-info">{health?.version || '0.1.0-dev'}</span>
          </div>
          {health?.components &&
            Object.entries(health.components).map(([key, val]) => (
              <div key={key} className="health-tile">
                <span className="tile-label">{key}</span>
                <span
                  className={`badge ${
                    val.status === 'UP' ? 'badge-success' : 'badge-warning'
                  }`}
                >
                  {val.status}
                </span>
              </div>
            ))}
        </div>
      )}
    </div>
  );
};
