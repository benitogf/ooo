function ExplorerApp() {
  const { useState, useEffect, useCallback } = React;
  const AppNav = window.AppNav;
  const Breadcrumb = window.Breadcrumb;
  const StorageList = window.StorageList;
  const StorageKeys = window.StorageKeys;
  const KeyEditor = window.KeyEditor;
  const StoragePush = window.StoragePush;
  const SettingsPage = window.SettingsPage;
  
  const [route, setRoute] = useState({ page: 'storage', keyPath: '', filterPath: '', keyType: '', fromFilter: '' });
  const [filterCount, setFilterCount] = useState(0);
  const [refreshCounter, setRefreshCounter] = useState(0);
  const [serverName, setServerName] = useState('');

  useEffect(() => {
    fetch('/?api=info')
      .then(res => res.json())
      .then(data => setServerName(data.name || ''))
      .catch(() => {});
  }, []);

  const parseHash = useCallback(() => {
    const hash = window.location.hash.slice(1) || '/storage';
    const [path, query] = hash.split('?');
    const params = new URLSearchParams(query || '');
    const fromFilter = params.get('from') || '';

    if (path === '/settings') {
      return { page: 'settings', keyPath: '', filterPath: '', keyType: '', fromFilter: '' };
    } else if (path.startsWith('/storage/push/')) {
      const filterPath = decodeURIComponent(path.replace('/storage/push/', ''));
      return { page: 'push', keyPath: '', filterPath, keyType: '', fromFilter: '' };
    } else if (path.startsWith('/storage/key/glob/')) {
      const filterPath = decodeURIComponent(path.replace('/storage/key/glob/', ''));
      return { page: 'keys', keyPath: '', filterPath, keyType: 'glob', fromFilter };
    } else if (path.startsWith('/storage/key/static/')) {
      const keyPath = decodeURIComponent(path.replace('/storage/key/static/', ''));
      return { page: 'editor', keyPath, filterPath: fromFilter, keyType: 'static', fromFilter };
    } else {
      return { page: 'storage', keyPath: '', filterPath: '', keyType: '', fromFilter: '' };
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
      .catch(() => {});
  }, [route.page]);

  const navigate = (path) => {
    window.location.hash = path;
  };

  const getActiveTab = () => {
    if (route.page === 'settings') return 'settings';
    return 'storage';
  };

  const getBreadcrumb = () => {
    switch (route.page) {
      case 'settings': return 'Settings';
      case 'keys': return route.filterPath;
      case 'editor': return route.keyPath;
      case 'push': return 'Push to ' + route.filterPath;
      default: return 'Storage';
    }
  };

  return (
    <div>
      <AppNav appName={serverName} activeTab={getActiveTab()} filterCount={filterCount} onNavigate={navigate} />
      <Breadcrumb current={getBreadcrumb()} />
      {route.page === 'storage' && <StorageList />}
      {route.page === 'keys' && <StorageKeys filterPath={route.filterPath} refresh={refreshCounter} />}
      {route.page === 'editor' && <KeyEditor keyPath={route.keyPath} filterPath={route.filterPath} isCreate={false} />}
      {route.page === 'push' && <StoragePush filterPath={route.filterPath} />}
      {route.page === 'settings' && <SettingsPage />}
    </div>
  );
}

window.ExplorerApp = ExplorerApp;
