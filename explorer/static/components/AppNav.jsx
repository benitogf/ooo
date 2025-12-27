function AppNav({ appName, activeTab, filterCount, onNavigate }) {
  const { IconBox, IconDatabase, IconSettings } = window.Icons;
  
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
        <button 
          className={`nav-tab ${activeTab === 'settings' ? 'active' : ''}`}
          onClick={() => onNavigate('/settings')}
        >
          <IconSettings />
          Settings
        </button>
      </div>
      <div className="nav-right">
        <div className="status-badge">
          <span className="status-dot"></span>
          ONLINE
        </div>
      </div>
    </nav>
  );
}

window.AppNav = AppNav;
