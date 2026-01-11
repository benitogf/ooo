function ExplorerApp() {
  const { useState, useEffect, useCallback } = React;
  const AppNav = window.AppNav;
  const Breadcrumb = window.Breadcrumb;
  const StorageList = window.StorageList;
  const StorageKeysLive = window.StorageKeysLive;
  const KeyEditor = window.KeyEditor;
  const KeyEditorLive = window.KeyEditorLive;
  const StoragePush = window.StoragePush;
  const StateModal = window.StateModal;
  const PivotStatus = window.PivotStatus;
  const EndpointsList = window.EndpointsList;
  const ProxiesList = window.ProxiesList;
  const ProxyView = window.ProxyView;
  const ProxyListView = window.ProxyListView;
  const OrphanKeysList = window.OrphanKeysList;

  const [route, setRoute] = useState({ page: 'storage', keyPath: '', filterPath: '', keyType: '', fromFilter: '', liveMode: false, source: '', canRead: true, canWrite: false, canDelete: false });
  const [filterCount, setFilterCount] = useState(0);
  const [refreshCounter, setRefreshCounter] = useState(0);
  const [serverName, setServerName] = useState('');
  const [isConnected, setIsConnected] = useState(true);
  const [stateModalOpen, setStateModalOpen] = useState(false);
  const [pivotModalOpen, setPivotModalOpen] = useState(false);

  const prevConnected = React.useRef(true);
  
  const handleConnectionChange = useCallback((connected) => {
    setIsConnected(connected);
  }, []);

  useEffect(() => {
    fetch('/?api=info')
      .then(res => res.json())
      .then(data => setServerName(data.name || ''))
      .catch(() => { });
  }, []);

  const parseHash = useCallback(() => {
    const hash = window.location.hash.slice(1) || '/storage';
    const [path, query] = hash.split('?');
    const params = new URLSearchParams(query || '');
    const fromFilter = params.get('from') || '';
    const source = params.get('source') || '';

    if (path === '/endpoints') {
      return { page: 'endpoints', keyPath: '', filterPath: '', keyType: '', fromFilter: '', liveMode: false, source: '', canRead: true, canWrite: false, canDelete: false };
    } else if (path === '/proxies') {
      return { page: 'proxies', keyPath: '', filterPath: '', keyType: '', fromFilter: '', liveMode: false, source: '', canRead: true, canWrite: false, canDelete: false };
    } else if (path === '/orphans') {
      return { page: 'orphans', keyPath: '', filterPath: '', keyType: '', fromFilter: '', liveMode: false, source: '', canRead: true, canWrite: false, canDelete: false };
    } else if (path.startsWith('/proxy/view/')) {
      const keyPath = decodeURIComponent(path.replace('/proxy/view/', ''));
      const canRead = params.get('canRead') === '1';
      const canWrite = params.get('canWrite') === '1';
      const canDelete = params.get('canDelete') === '1';
      const mode = params.get('mode') || 'live';
      const proxyType = params.get('type') || 'single';
      const page = proxyType === 'list' ? 'proxyListView' : 'proxyView';
      return { page, keyPath, filterPath: '', keyType: proxyType, fromFilter: params.get('from') || '', liveMode: mode === 'live', source: 'proxies', canRead, canWrite, canDelete };
    } else if (path.startsWith('/storage/push/')) {
      const filterPath = decodeURIComponent(path.replace('/storage/push/', ''));
      return { page: 'push', keyPath: '', filterPath, keyType: '', fromFilter: '', liveMode: false, source };
    } else if (path.startsWith('/storage/keys/live/')) {
      const filterPath = decodeURIComponent(path.replace('/storage/keys/live/', ''));
      return { page: 'keys', keyPath: '', filterPath, keyType: 'glob', fromFilter, source };
    } else if (path.startsWith('/storage/keys/static/')) {
      // Redirect static to live
      const filterPath = decodeURIComponent(path.replace('/storage/keys/static/', ''));
      window.location.hash = '/storage/keys/live/' + encodeURIComponent(filterPath) + (source ? '?source=' + source : '');
      return { page: 'keys', keyPath: '', filterPath, keyType: 'glob', fromFilter, source };
    } else if (path.startsWith('/storage/key/live/')) {
      const keyPath = decodeURIComponent(path.replace('/storage/key/live/', ''));
      return { page: 'editor', keyPath, filterPath: fromFilter, keyType: 'static', fromFilter, liveMode: true, source };
    } else if (path.startsWith('/storage/key/static/')) {
      const keyPath = decodeURIComponent(path.replace('/storage/key/static/', ''));
      return { page: 'editor', keyPath, filterPath: fromFilter, keyType: 'static', fromFilter, liveMode: false, source };
    } else if (path.startsWith('/storage/key/glob/')) {
      const filterPath = decodeURIComponent(path.replace('/storage/key/glob/', ''));
      return { page: 'keys', keyPath: '', filterPath, keyType: 'glob', fromFilter, liveMode: false, source };
    } else {
      return { page: 'storage', keyPath: '', filterPath: '', keyType: '', fromFilter: '', liveMode: false, source: '', canRead: true, canWrite: false, canDelete: false };
    }
  }, []);

  useEffect(() => {
    const handleHashChange = () => {
      setRoute(parseHash());
      setRefreshCounter(c => c + 1);
    };
    window.addEventListener('hashchange', handleHashChange);
    handleHashChange();
    return () => window.removeEventListener('hashchange', handleHashChange);
  }, [parseHash]);

  // Fetch filter count (uses data.filters which excludes pivot-prefixed paths)
  const loadFilterCount = useCallback(() => {
    fetch('/?api=filters')
      .then(res => res.json())
      .then(data => setFilterCount((data.filters || data.paths || []).length))
      .catch(() => { });
  }, []);

  useEffect(() => {
    loadFilterCount();
  }, [route.page, loadFilterCount]);

  // Reload filter count when clock reconnects
  useEffect(() => {
    if (isConnected && !prevConnected.current) {
      loadFilterCount();
    }
    prevConnected.current = isConnected;
  }, [isConnected, loadFilterCount]);

  const navigate = (path) => {
    window.location.hash = path;
  };

  const getActiveTab = () => {
    // If navigated from proxies, keep proxies tab active
    if (route.source === 'proxies' || route.page === 'proxyView' || route.page === 'proxyListView') return 'proxies';
    switch (route.page) {
      case 'endpoints': return 'endpoints';
      case 'proxies': return 'proxies';
      case 'orphans': return 'orphans';
      default: return 'storage';
    }
  };

  const toggleStateModal = () => {
    setStateModalOpen(prev => !prev);
  };

  const togglePivotModal = () => {
    setPivotModalOpen(prev => !prev);
  };

  const getBreadcrumb = () => {
    const livePrefix = route.liveMode ? 'Live: ' : '';
    switch (route.page) {
      case 'keys': return livePrefix + route.filterPath;
      case 'editor': return livePrefix + route.keyPath;
      case 'push': return 'Push to ' + route.filterPath;
      case 'endpoints': return 'Custom Endpoints';
      case 'proxies': return 'Proxy Routes';
      case 'proxyView': return 'Proxy: ' + route.keyPath;
      case 'proxyListView': return 'Proxy List: ' + route.keyPath;
      case 'orphans': return 'Orphan Keys';
      default: return 'Filters';
    }
  };

  return (
    <div className={isConnected ? '' : 'app-offline'}>
      <AppNav
        appName={serverName}
        activeTab={getActiveTab()}
        filterCount={filterCount}
        onNavigate={navigate}
        onConnectionChange={handleConnectionChange}
        onStateClick={toggleStateModal}
        stateModalOpen={stateModalOpen}
        onPivotClick={togglePivotModal}
        pivotModalOpen={pivotModalOpen}
      />
      <Breadcrumb current={getBreadcrumb()} />
      <div className="app-content">
        {route.page === 'storage' && <StorageList clockConnected={isConnected} />}
        {route.page === 'keys' && <StorageKeysLive filterPath={route.filterPath} fromFilter={route.fromFilter} source={route.source} />}
        {route.page === 'editor' && !route.liveMode && <KeyEditor keyPath={route.keyPath} filterPath={route.filterPath} isCreate={false} />}
        {route.page === 'editor' && route.liveMode && <KeyEditorLive keyPath={route.keyPath} fromFilter={route.fromFilter} source={route.source} />}
        {route.page === 'push' && <StoragePush filterPath={route.filterPath} />}
        {route.page === 'endpoints' && <EndpointsList clockConnected={isConnected} />}
        {route.page === 'proxies' && <ProxiesList clockConnected={isConnected} />}
        {route.page === 'proxyView' && <ProxyView keyPath={route.keyPath} canRead={route.canRead} canWrite={route.canWrite} canDelete={route.canDelete} liveMode={route.liveMode} fromFilter={route.fromFilter} />}
        {route.page === 'proxyListView' && <ProxyListView proxyPath={route.keyPath} canRead={route.canRead} canWrite={route.canWrite} canDelete={route.canDelete} />}
        {route.page === 'orphans' && <OrphanKeysList clockConnected={isConnected} />}
      </div>

      <StateModal
        visible={stateModalOpen}
        onClose={() => setStateModalOpen(false)}
      />
      {pivotModalOpen && (
        <div className="modal-overlay" onClick={() => setPivotModalOpen(false)}>
          <div onClick={e => e.stopPropagation()}>
            <PivotStatus onClose={() => setPivotModalOpen(false)} />
          </div>
        </div>
      )}
      {!isConnected && (
        <div className="offline-overlay">
          <div className="offline-message">
            <span className="offline-icon">⚠</span>
            <span>Server connection lost. Reconnecting...</span>
          </div>
        </div>
      )}
    </div>
  );
}

window.ExplorerApp = ExplorerApp;
