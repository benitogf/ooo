function StateModal({ visible, onClose }) {
  const { useState, useEffect, useCallback } = React;
  const { IconActivity, IconRefresh, IconX } = window.Icons;
  
  const [state, setState] = useState({ pools: [], totalConnections: 0 });
  const [info, setInfo] = useState([]);
  const [loading, setLoading] = useState(false);
  const [infoExpanded, setInfoExpanded] = useState(false);

  const formatValue = (val) => {
    if (typeof val === 'boolean') return val ? 'Yes' : 'No';
    if (typeof val === 'number' && val > 1000000) return (val / 1000000000) + 's';
    return String(val);
  };

  const loadData = useCallback(async () => {
    setLoading(true);
    try {
      const [stateRes, infoRes] = await Promise.all([
        fetch('/?api=state'),
        fetch('/?api=info')
      ]);
      const stateData = await stateRes.json();
      const infoData = await infoRes.json();
      setState(stateData);
      setInfo(Object.entries(infoData).map(([key, value]) => ({
        key,
        value: formatValue(value)
      })));
    } catch (err) {
      console.error('Failed to load data:', err);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (visible) {
      loadData();
    }
  }, [visible, loadData]);

  if (!visible) return null;

  const handleOverlayClick = (e) => {
    if (e.target === e.currentTarget) onClose();
  };

  return (
    <div className="modal-overlay" onClick={handleOverlayClick}>
      <div className="modal state-modal" onClick={(e) => e.stopPropagation()}>
        <div className="state-modal-header">
          <div className="state-modal-title">
            <IconActivity />
            Server State
          </div>
          <div className="state-modal-actions">
            <button className="refresh-btn" onClick={loadData} title="Refresh">
              <IconRefresh />
            </button>
            <button className="close-btn" onClick={onClose} title="Close">
              <IconX />
            </button>
          </div>
        </div>

        <div className={`state-modal-content ${loading ? 'loading' : ''}`}>
          {/* Server Info Section */}
          <div className="state-section">
            <div className="state-section-title">Server Info</div>
            <div className="info-row">
              {info.filter(item => item.key === 'name' || item.key === 'address').map(item => (
                <div key={item.key} className="info-chip primary">
                  <span className="info-chip-label">{item.key}:</span>
                  <span className="info-chip-value">{item.value}</span>
                </div>
              ))}
              {infoExpanded && info.filter(item => item.key !== 'name' && item.key !== 'address').map(item => (
                <div key={item.key} className="info-chip">
                  <span className="info-chip-label">{item.key}:</span>
                  <span className="info-chip-value">{item.value}</span>
                </div>
              ))}
              <button 
                className={`info-expand-btn ${infoExpanded ? 'expanded' : ''}`}
                onClick={() => setInfoExpanded(!infoExpanded)}
              >
                {infoExpanded ? '− Less' : `+ ${info.length - 2} more`}
              </button>
            </div>
          </div>

          {/* Stream State Section */}
          <div className="state-section">
            <div className="state-section-title">
              Stream Connections
              <span className="state-stats">
                <span className="stat-pill">{state.pools?.length || 0} pools</span>
                <span className="stat-pill">{state.totalConnections || 0} connections</span>
              </span>
            </div>
            <div className="state-table-container">
              <table className="data-table compact">
                <thead>
                  <tr>
                    <th>Key</th>
                    <th style={{ textAlign: 'right' }}>Connections</th>
                  </tr>
                </thead>
                <tbody>
                  {(!state.pools || state.pools.length === 0) ? (
                    <tr>
                      <td colSpan="2">
                        <div className="empty-state small">
                          <div>No active connections</div>
                        </div>
                      </td>
                    </tr>
                  ) : (
                    state.pools.map(pool => (
                      <tr key={pool.key}>
                        <td><span className="key-path">{pool.key}</span></td>
                        <td style={{ textAlign: 'right' }}>
                          <span className="connection-count">{pool.connections}</span>
                        </td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
          </div>
        </div>

      </div>
    </div>
  );
}

window.StateModal = StateModal;
