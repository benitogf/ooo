function AppNav({ appName, activeTab, filterCount, onNavigate, onConnectionChange, onStateClick, stateModalOpen, onPivotClick, pivotModalOpen }) {
  const { useState, useEffect, useRef } = React;
  const { IconBox, IconDatabase, IconActivity, IconServer, IconCloud, IconCloudOff, IconCode, IconLink, IconWarning } = window.Icons;
  const StateModal = window.StateModal;
  const [endpointCount, setEndpointCount] = useState(0);
  const [proxyCount, setProxyCount] = useState(0);
  const [pivotRole, setPivotRole] = useState(null);
  const prevConnected = useRef(false);

  // Use ooo-client for clock subscription
  const { time: serverTime, connected: clockConnected } = Api.useSubscribeClock();

  useEffect(() => {
    // Notify parent of connection state changes
    if (onConnectionChange && prevConnected.current !== clockConnected) {
      prevConnected.current = clockConnected;
      onConnectionChange(clockConnected);
    }
  }, [clockConnected, onConnectionChange]);

  // Reload all config data when clock reconnects (configs only change after server restart)
  useEffect(() => {
    if (!clockConnected) return;
    
    // Fetch pivot role
    fetch('/?api=pivot')
      .then(res => res.json())
      .then(data => setPivotRole(data.role || 'none'))
      .catch(() => setPivotRole('none'));
    
    // Fetch counts for navigation badges (endpoints/proxies only change on restart)
    Api.fetchEndpoints().then(data => setEndpointCount((data || []).length)).catch(() => {});
    Api.fetchProxies().then(data => setProxyCount((data || []).length)).catch(() => {});
  }, [clockConnected]);

  const getPivotIcon = () => {
    switch (pivotRole) {
      case 'pivot': return <IconServer />;
      case 'node': return <IconCloud />;
      default: return <IconCloudOff />;
    }
  };

  const formatTime = (date) => {
    if (!date) return '--:--:--';
    return date.toLocaleTimeString();
  };

  return (
    <nav className="top-nav">
      <div className="logo">
        <div className="logo-icon"><IconBox /></div>
        <span>{appName}</span>
      </div>
      <div className="nav-tabs" style={{ visibility: activeTab ? 'visible' : 'hidden' }}>
        <button
          className={`nav-tab ${activeTab === 'storage' ? 'active' : ''}`}
          onClick={() => onNavigate('/storage')}
        >
          <IconDatabase />
          Filters
          <span className="badge">{filterCount}</span>
        </button>
        {endpointCount > 0 && (
          <button
            className={`nav-tab ${activeTab === 'endpoints' ? 'active' : ''}`}
            onClick={() => onNavigate('/endpoints')}
          >
            <IconCode />
            Endpoints
            <span className="badge">{endpointCount}</span>
          </button>
        )}
        {proxyCount > 0 && (
          <button
            className={`nav-tab ${activeTab === 'proxies' ? 'active' : ''}`}
            onClick={() => onNavigate('/proxies')}
          >
            <IconLink />
            Proxies
            <span className="badge">{proxyCount}</span>
          </button>
        )}
        <button
          className={`nav-tab ${activeTab === 'orphans' ? 'active' : ''}`}
          onClick={() => onNavigate('/orphans')}
        >
          <IconWarning />
          Orphans
        </button>
      </div>
      <div className="nav-right">
        {pivotRole && (
          <button
            className={`pivot-btn ${pivotModalOpen ? 'active' : ''}`}
            onClick={onPivotClick}
            title="Pivot Status"
          >
            {getPivotIcon()}
            <span className={`pivot-role-badge ${pivotRole}`}>{pivotRole}</span>
          </button>
        )}
        <button
          className={`state-btn ${stateModalOpen ? 'active' : ''}`}
          onClick={onStateClick}
          title="Server State"
        >
          <IconActivity />
        </button>
        <div className={`status-badge ${clockConnected ? '' : 'offline'}`}>
          <span className="status-dot"></span>
          <span className="server-time">{formatTime(serverTime)}</span>
          {clockConnected ? 'ONLINE' : 'OFFLINE'}
        </div>
      </div>
    </nav>
  );
}

window.AppNav = AppNav;
