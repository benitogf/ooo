function AppNav({ appName, activeTab, filterCount, onNavigate, onConnectionChange, onStateClick, stateModalOpen, onPivotClick, pivotModalOpen }) {
  const { useState, useEffect, useRef } = React;
  const { IconBox, IconDatabase, IconActivity, IconServer, IconCloud, IconCloudOff } = window.Icons;
  const StateModal = window.StateModal;

  const [serverTime, setServerTime] = useState(null);
  const [clockConnected, setClockConnected] = useState(false);
  const [pivotRole, setPivotRole] = useState(null);
  const prevConnected = useRef(false);

  useEffect(() => {
    // Notify parent of connection state changes
    if (onConnectionChange && prevConnected.current !== clockConnected) {
      prevConnected.current = clockConnected;
      onConnectionChange(clockConnected);
    }
  }, [clockConnected, onConnectionChange]);

  useEffect(() => {
    // Fetch pivot role on mount
    fetch('/?api=pivot')
      .then(res => res.json())
      .then(data => setPivotRole(data.role || 'none'))
      .catch(() => setPivotRole('none'));
  }, []);

  const getPivotIcon = () => {
    switch (pivotRole) {
      case 'pivot': return <IconServer />;
      case 'node': return <IconCloud />;
      default: return <IconCloudOff />;
    }
  };

  useEffect(() => {
    // Subscribe to server clock using native WebSocket
    // ooo-client has issues with clock mode, so we use raw WebSocket
    const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${wsProtocol}//${window.location.host}`;
    let ws = null;
    let reconnectTimeout = null;

    const connect = () => {
      ws = new WebSocket(wsUrl);
      ws.binaryType = 'arraybuffer';

      ws.onopen = () => {
        setClockConnected(true);
      };

      ws.onmessage = (event) => {
        try {
          const decoder = new TextDecoder('utf8');
          const text = decoder.decode(event.data);
          const timestamp = parseInt(text, 10);
          if (!isNaN(timestamp)) {
            setServerTime(new Date(timestamp / 1000000));
          }
        } catch (e) {
          console.warn('Clock parse error:', e);
        }
      };

      ws.onclose = () => {
        setClockConnected(false);
        setServerTime(null); // Clear stale time when disconnected
        // Reconnect after 3 seconds
        reconnectTimeout = setTimeout(connect, 3000);
      };

      ws.onerror = () => {
        setClockConnected(false);
        setServerTime(null); // Clear stale time on error
      };
    };

    connect();

    return () => {
      if (reconnectTimeout) clearTimeout(reconnectTimeout);
      if (ws) {
        ws.close();
        ws = null;
      }
    };
  }, []);

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
          Storage
          <span className="badge">{filterCount}</span>
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
