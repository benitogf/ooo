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

  const [route, setRoute] = useState({ page: 'storage', keyPath: '', filterPath: '', keyType: '', fromFilter: '', liveMode: false });
  const [filterCount, setFilterCount] = useState(0);
  const [refreshCounter, setRefreshCounter] = useState(0);
  const [serverName, setServerName] = useState('');
  const [isConnected, setIsConnected] = useState(true);
  const [stateModalOpen, setStateModalOpen] = useState(false);
  const [pivotModalOpen, setPivotModalOpen] = useState(false);

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

    if (path.startsWith('/storage/push/')) {
      const filterPath = decodeURIComponent(path.replace('/storage/push/', ''));
      return { page: 'push', keyPath: '', filterPath, keyType: '', fromFilter: '', liveMode: false };
    } else if (path.startsWith('/storage/keys/live/')) {
      const filterPath = decodeURIComponent(path.replace('/storage/keys/live/', ''));
      return { page: 'keys', keyPath: '', filterPath, keyType: 'glob', fromFilter };
    } else if (path.startsWith('/storage/keys/static/')) {
      // Redirect static to live
      const filterPath = decodeURIComponent(path.replace('/storage/keys/static/', ''));
      window.location.hash = '/storage/keys/live/' + encodeURIComponent(filterPath);
      return { page: 'keys', keyPath: '', filterPath, keyType: 'glob', fromFilter };
    } else if (path.startsWith('/storage/key/live/')) {
      const keyPath = decodeURIComponent(path.replace('/storage/key/live/', ''));
      return { page: 'editor', keyPath, filterPath: fromFilter, keyType: 'static', fromFilter, liveMode: true };
    } else if (path.startsWith('/storage/key/static/')) {
      const keyPath = decodeURIComponent(path.replace('/storage/key/static/', ''));
      return { page: 'editor', keyPath, filterPath: fromFilter, keyType: 'static', fromFilter, liveMode: false };
    } else if (path.startsWith('/storage/key/glob/')) {
      const filterPath = decodeURIComponent(path.replace('/storage/key/glob/', ''));
      return { page: 'keys', keyPath: '', filterPath, keyType: 'glob', fromFilter, liveMode: false };
    } else {
      return { page: 'storage', keyPath: '', filterPath: '', keyType: '', fromFilter: '', liveMode: false };
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

  useEffect(() => {
    fetch('/?api=filters')
      .then(res => res.json())
      .then(data => setFilterCount((data.paths || []).length))
      .catch(() => { });
  }, [route.page]);

  const navigate = (path) => {
    window.location.hash = path;
  };

  const getActiveTab = () => {
    return 'storage';
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
      default: return 'Storage';
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
        {route.page === 'storage' && <StorageList />}
        {route.page === 'keys' && <StorageKeysLive filterPath={route.filterPath} fromFilter={route.fromFilter} />}
        {route.page === 'editor' && !route.liveMode && <KeyEditor keyPath={route.keyPath} filterPath={route.filterPath} isCreate={false} />}
        {route.page === 'editor' && route.liveMode && <KeyEditorLive keyPath={route.keyPath} fromFilter={route.fromFilter} />}
        {route.page === 'push' && <StoragePush filterPath={route.filterPath} />}
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
